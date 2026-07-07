package web

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/fsnotify/fsnotify"
)

// sseHub manages SSE client connections and broadcasts state updates.
type sseHub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{
		clients: make(map[chan []byte]struct{}),
	}
}

func (h *sseHub) register() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) unregister(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

func (h *sseHub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// Skip slow clients
		}
	}
}

// StartWatcher watches the agents directory and broadcasts state changes
// to SSE clients.
func (s *Server) StartWatcher() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watchDir := state.AgentsDir(s.cfg.Profile.StateDir)
	_ = os.MkdirAll(watchDir, 0700)
	if err := watcher.Add(watchDir); err != nil {
		watcher.Close()
		return nil, err
	}

	go func() {
		var debounce *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					if debounce != nil {
						debounce.Stop()
					}
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					if debounce != nil {
						debounce.Stop()
					}
					// 100ms was too eager when several codex agents wrote
					// state files concurrently — the timer kept resetting
					// then fired a burst of resolutions. 300ms still feels
					// instant in an SSE feed but lets a single tick absorb
					// the burst.
					debounce = time.AfterFunc(300*time.Millisecond, func() {
						agents := s.readAgentState()
						data, err := json.Marshal(agents)
						if err != nil {
							return
						}
						s.hub.broadcast(data)
					})
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return watcher, nil
}

// readAgentState reads and resolves the current agent state. Concurrent
// callers (debounced fsnotify firings + ad-hoc REST handlers) coalesce
// via singleflight so a burst of state writes only triggers one tmux +
// git resolution pass.
func (s *Server) readAgentState() []domain.Agent {
	v, _, _ := s.readAgentStateGroup.Do("readAgentState", func() (any, error) {
		agents := state.ResolveChain(state.ResolveOptions{
			StateDir:          s.cfg.Profile.StateDir,
			ClaudeProjectsDir: s.cfg.Profile.ProjectsDir,
			ClaudeSessionsDir: s.cfg.Profile.SessionsDir,
			CodexSessionsDir:  s.codexSessionsRootDir,
			TmuxAvailable:     tmux.TmuxIsAvailable(),
			Store:             s.store,
		})
		return s.applyTrustFlags(agents), nil
	})
	agents, _ := v.([]domain.Agent)
	return agents
}

// pruneDeadOnce runs one prune/sweep cycle with the TUI's enumeration guard:
// an unknown server PID or empty pane set means the enumeration can't be
// trusted, so the cycle is skipped rather than treating every agent as dead.
// The merged-branch checker is nil here — the web surface skips merged-GC
// and relies on the resumable TTL backstop, avoiding a second git fanout.
func (s *Server) pruneDeadOnce() int {
	livePaneIDs, serverPID := tmux.TmuxListLivePaneIDs()
	if len(livePaneIDs) == 0 || serverPID == "" {
		return 0
	}
	return state.PruneDead(s.cfg.Profile.StateDir, livePaneIDs, serverPID, nil, s.store)
}

// StartPruneLoop gives web-only deployments the same periodic prune/sweep
// the TUI runs on its tick: without it, dead-pane files and orphaned
// read-model rows accumulate until a TUI happens to start. Runs once
// immediately, then on a long interval; stops when stop is closed.
func (s *Server) StartPruneLoop(stop <-chan struct{}) {
	go func() {
		_ = s.pruneDeadOnce()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.pruneDeadOnce()
			case <-stop:
				return
			}
		}
	}()
}
