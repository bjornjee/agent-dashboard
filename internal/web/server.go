package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

//go:embed static
var staticFS embed.FS

// ServerOptions configures the web server.
type ServerOptions struct {
	GoogleClientID     string
	GoogleClientSecret string
	AllowedEmail       string
	SessionSecret      string
}

// Server is the HTTP server for the web dashboard.
type Server struct {
	cfg  domain.Config
	db   *db.DB
	opts ServerOptions
	auth *authHandler
	hub  *sseHub

	// Cached codex sessions root ($CODEX_HOME/sessions or
	// ~/.codex/sessions). Resolved once at construction so request
	// handlers don't re-read env on every conversation fetch and tests
	// get a stable value for the Server's lifetime.
	codexSessionsRootDir string

	// Rate-limit cache (60s TTL to avoid per-request API calls)
	rlMu        sync.Mutex
	rlCache     *domain.RateLimit
	rlFetchedAt time.Time

	// Trust-prompt tracker: set of tmux pane_ids whose harness has shown a
	// folder-trust dialog post-spawn. Populated by watchTrustPrompt after
	// handleCreate, surfaced via readAgentState so SSE/REST clients see
	// TrustPromptDetected on the affected agent. Cleared by handleClose.
	trustMu    sync.Mutex
	trustPanes map[string]trustPaneRecord
}

// NewServer creates a new web dashboard server.
func NewServer(cfg domain.Config, database *db.DB, opts ServerOptions) *Server {
	s := &Server{
		cfg:                  cfg,
		db:                   database,
		opts:                 opts,
		hub:                  newSSEHub(),
		codexSessionsRootDir: resolveCodexSessionsRoot(cfg.Profile.HomeDir),
		trustPanes:           make(map[string]trustPaneRecord),
	}
	if opts.GoogleClientID != "" {
		s.auth = newAuthHandler(opts)
	}
	return s
}

// resolveCodexSessionsRoot returns $CODEX_HOME/sessions if set, else
// homeDir/.codex/sessions. Pure function so handlers and tests can rely
// on a stable value for the lifetime of the Server.
func resolveCodexSessionsRoot(homeDir string) string {
	if env := os.Getenv("CODEX_HOME"); env != "" {
		return filepath.Join(env, "sessions")
	}
	return filepath.Join(homeDir, ".codex", "sessions")
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Auth routes (always accessible)
	if s.auth != nil {
		mux.HandleFunc("GET /auth/login", s.auth.handleLogin)
		mux.HandleFunc("GET /auth/google", s.auth.handleGoogleRedirect)
		mux.HandleFunc("GET /auth/callback", s.auth.handleCallback)
		mux.HandleFunc("POST /auth/logout", s.requireCSRF(s.auth.handleLogout))
	}

	// Read API routes
	mux.HandleFunc("GET /api/agents", s.requireAuth(s.handleAgents))
	mux.HandleFunc("GET /api/agents/{id}", s.requireAuth(s.handleAgent))
	mux.HandleFunc("GET /api/agents/{id}/conversation", s.requireAuth(s.handleConversation))
	mux.HandleFunc("GET /api/agents/{id}/activity", s.requireAuth(s.handleActivity))
	mux.HandleFunc("GET /api/agents/{id}/diff", s.requireAuth(s.handleDiff))
	mux.HandleFunc("GET /api/agents/{id}/plan", s.requireAuth(s.handlePlan))
	mux.HandleFunc("GET /api/agents/{id}/pending-question", s.requireAuth(s.handlePendingQuestion))
	mux.HandleFunc("GET /api/agents/{id}/usage", s.requireAuth(s.handleUsage))
	mux.HandleFunc("GET /api/agents/{id}/subagents", s.requireAuth(s.handleSubagents))
	mux.HandleFunc("GET /api/agents/{id}/pr-url", s.requireAuth(s.handlePRURL))
	mux.HandleFunc("GET /api/usage/daily", s.requireAuth(s.handleDailyUsage))
	mux.HandleFunc("GET /api/usage/ratelimit", s.requireAuth(s.handleRateLimit))
	mux.HandleFunc("GET /api/skills", s.requireAuth(s.handleSkills))
	mux.HandleFunc("GET /api/harness-options", s.requireAuth(s.handleHarnessOptions))
	mux.HandleFunc("GET /api/suggestions", s.requireAuth(s.handleSuggestions))
	mux.HandleFunc("POST /api/file-picker", s.requireAuth(s.requireCSRF(s.handleFilePicker)))

	// Action routes (require session + CSRF header)
	mux.HandleFunc("POST /api/agents/{id}/approve", s.requireAuth(s.requireCSRF(s.handleApprove)))
	mux.HandleFunc("POST /api/agents/{id}/reject", s.requireAuth(s.requireCSRF(s.handleReject)))
	mux.HandleFunc("POST /api/agents/{id}/input", s.requireAuth(s.requireCSRF(s.handleInput)))
	mux.HandleFunc("POST /api/agents/{id}/answer-question", s.requireAuth(s.requireCSRF(s.handleAnswerQuestion)))
	mux.HandleFunc("POST /api/agents/{id}/stop", s.requireAuth(s.requireCSRF(s.handleStop)))
	mux.HandleFunc("POST /api/agents/{id}/close", s.requireAuth(s.requireCSRF(s.handleClose)))
	mux.HandleFunc("POST /api/agents/{id}/merge", s.requireAuth(s.requireCSRF(s.handleMerge)))
	mux.HandleFunc("POST /api/agents/{id}/cleanup", s.requireAuth(s.requireCSRF(s.handleCleanup)))
	mux.HandleFunc("POST /api/agents/{id}/resume", s.requireAuth(s.requireCSRF(s.handleResume)))
	mux.HandleFunc("POST /api/agents/create", s.requireAuth(s.requireCSRF(s.handleCreate)))

	// SSE endpoint
	mux.HandleFunc("GET /events", s.requireAuth(s.handleSSE))

	// Static files. In dev mode (DASHBOARD_DEV=1 or DASHBOARD_DEV_STATIC=<path>),
	// serve from disk with no-cache headers so CSS/JS/HTML edits show up on
	// reload without rebuilding the binary. Otherwise serve the embed.FS
	// snapshot baked into the binary.
	mux.Handle("GET /", s.staticHandler())

	return mux
}

// staticHandler returns the file-server handler for the embedded UI.
// Honours DASHBOARD_DEV_STATIC (absolute path) and DASHBOARD_DEV=1
// (auto-detect `internal/web/static` relative to CWD) for hot-reload dev.
func (s *Server) staticHandler() http.Handler {
	if devDir := devStaticDir(); devDir != "" {
		log.Printf("dev mode: serving static files from disk: %s", devDir)
		return noCache(http.FileServer(http.Dir(devDir)))
	}
	staticSub, _ := fs.Sub(staticFS, "static")
	return http.FileServer(http.FS(staticSub))
}

// devStaticDir returns the on-disk dev path for static assets, or "" if
// dev mode is not enabled / the path does not exist.
func devStaticDir() string {
	if p := os.Getenv("DASHBOARD_DEV_STATIC"); p != "" {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
		log.Printf("dev mode: DASHBOARD_DEV_STATIC=%q not a directory; falling back to embed", p)
	}
	if os.Getenv("DASHBOARD_DEV") == "1" {
		if info, err := os.Stat("internal/web/static"); err == nil && info.IsDir() {
			return "internal/web/static"
		}
		log.Printf("dev mode: DASHBOARD_DEV=1 set but ./internal/web/static not found; falling back to embed")
	}
	return ""
}

// noCache disables HTTP and browser-side caching on the wrapped handler.
// Used in dev mode so CSS/JS/HTML edits show up on the next reload.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		h.ServeHTTP(w, r)
	})
}

// requireAuth wraps a handler with authentication if auth is configured.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth != nil {
			email, ok := s.auth.validateSession(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if email != s.opts.AllowedEmail {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next(w, r)
	}
}

// requireCSRF checks for the X-Requested-With header on POST requests.
func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "dashboard" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-Requested-With header"})
			return
		}
		next(w, r)
	}
}

// createSessionToken is exposed for testing.
func (s *Server) createSessionToken(email string) (string, error) {
	if s.auth == nil {
		return "", nil
	}
	return s.auth.createToken(email)
}

// handleAgents returns all agents sorted by state priority.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.readAgentState()
	writeJSON(w, http.StatusOK, agents)
}

// handleAgent returns a single agent by session ID.
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, ok := s.lookupAgent(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

// lookupAgent finds an agent by session ID from the current state.
func (s *Server) lookupAgent(id string) (domain.Agent, bool) {
	sf := state.ReadState(s.cfg.Profile.StateDir)
	var paneCwds map[string]string
	if tmux.TmuxIsAvailable() {
		targets, cwds, _ := tmux.TmuxListPanes()
		state.ResolveAgentTargets(&sf, targets)
		paneCwds = cwds
	}
	state.ResolveAgentProjDir(&sf, s.cfg.Profile.ProjectsDir, s.cfg.Profile.SessionsDir)
	state.ApplySpawnPins(&sf, s.cfg.Profile.StateDir)
	state.ResolveAgentWorktree(&sf, s.cfg.Profile.StateDir)
	state.ResolveAgentBranches(&sf, paneCwds, s.cfg.Profile.StateDir)
	state.ApplyStateArbitration(&sf, s.codexSessionsRootDir)
	agent, ok := sf.Agents[id]
	return agent, ok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
