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

// StartWatcher watches the agents directory and broadcasts state changes to SSE clients.
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
					debounce = time.AfterFunc(100*time.Millisecond, func() {
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

// readAgentState reads and resolves the current agent state.
func (s *Server) readAgentState() []domain.Agent {
	sf := state.ReadState(s.cfg.Profile.StateDir)
	if tmux.TmuxIsAvailable() {
		state.ResolveAgentTargets(&sf, tmux.TmuxListPaneTargets())
		state.ResolveAgentBranches(&sf, tmux.TmuxListPaneCwds())
	}
	state.ApplyPinnedStates(&sf)
	state.ApplyIdleOverrides(&sf, s.cfg.Profile.ProjectsDir)
	return state.SortedAgents(sf, "")
}
