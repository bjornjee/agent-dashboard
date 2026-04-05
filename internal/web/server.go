package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

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
}

// NewServer creates a new web dashboard server.
func NewServer(cfg domain.Config, database *db.DB, opts ServerOptions) *Server {
	s := &Server{
		cfg:  cfg,
		db:   database,
		opts: opts,
		hub:  newSSEHub(),
	}
	if opts.GoogleClientID != "" {
		s.auth = newAuthHandler(opts)
	}
	return s
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
	mux.HandleFunc("GET /api/agents/{id}/usage", s.requireAuth(s.handleUsage))
	mux.HandleFunc("GET /api/agents/{id}/subagents", s.requireAuth(s.handleSubagents))
	mux.HandleFunc("GET /api/usage/daily", s.requireAuth(s.handleDailyUsage))
	mux.HandleFunc("GET /api/suggestions", s.requireAuth(s.handleSuggestions))

	// Action routes (require session + CSRF header)
	mux.HandleFunc("POST /api/agents/{id}/approve", s.requireAuth(s.requireCSRF(s.handleApprove)))
	mux.HandleFunc("POST /api/agents/{id}/reject", s.requireAuth(s.requireCSRF(s.handleReject)))
	mux.HandleFunc("POST /api/agents/{id}/input", s.requireAuth(s.requireCSRF(s.handleInput)))
	mux.HandleFunc("POST /api/agents/{id}/stop", s.requireAuth(s.requireCSRF(s.handleStop)))
	mux.HandleFunc("POST /api/agents/{id}/close", s.requireAuth(s.requireCSRF(s.handleClose)))
	mux.HandleFunc("POST /api/agents/{id}/merge", s.requireAuth(s.requireCSRF(s.handleMerge)))
	mux.HandleFunc("POST /api/agents/create", s.requireAuth(s.requireCSRF(s.handleCreate)))

	// SSE endpoint
	mux.HandleFunc("GET /events", s.requireAuth(s.handleSSE))

	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /", http.FileServer(http.FS(staticSub)))

	return mux
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
	sf := state.ReadState(s.cfg.Profile.StateDir)
	agent, ok := sf.Agents[id]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

// lookupAgent finds an agent by session ID from the current state.
func (s *Server) lookupAgent(id string) (domain.Agent, bool) {
	sf := state.ReadState(s.cfg.Profile.StateDir)
	if tmux.TmuxIsAvailable() {
		state.ResolveAgentTargets(&sf, tmux.TmuxListPaneTargets())
	}
	state.ApplyPinnedStates(&sf)
	agent, ok := sf.Agents[id]
	return agent, ok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
