package web

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/singleflight"
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
	v, _, _ := readAgentStateGroup.Do("readAgentState", func() (any, error) {
		sf := state.ReadState(s.cfg.Profile.StateDir)
		var paneCwds map[string]string
		if tmux.TmuxIsAvailable() {
			targets, cwds := tmux.TmuxListPanes()
			state.ResolveAgentTargets(&sf, targets)
			paneCwds = cwds
		}
		state.ResolveAgentProjDir(&sf, s.cfg.Profile.ProjectsDir, s.cfg.Profile.SessionsDir)
		// Apply spawn-pins BEFORE marker-scan so freshly-spawned agents
		// render with the dashboard-staged pin before the JS hook fires.
		state.ApplySpawnPins(&sf, s.cfg.Profile.StateDir)
		state.ResolveAgentWorktree(&sf, s.cfg.Profile.StateDir)
		state.ResolveAgentBranches(&sf, paneCwds, s.cfg.Profile.StateDir)
		state.GCSpawnPins(s.cfg.Profile.StateDir, 10*time.Minute)
		state.ApplyPinnedStates(&sf)
		state.ApplyIdleOverrides(&sf, s.codexSessionsRootDir)
		agents := conversation.TopLevelAgents(
			state.SortedAgents(sf, ""),
			conversation.Roots{CodexSessionsRoot: s.codexSessionsRootDir},
		)
		return s.applyTrustFlags(agents), nil
	})
	agents, _ := v.([]domain.Agent)
	return agents
}

var readAgentStateGroup singleflight.Group
