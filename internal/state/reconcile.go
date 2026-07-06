package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	codexthreads "github.com/bjornjee/agent-dashboard/internal/codex/threads"
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// ReconcileUnregistered injects transient rows for live harness panes that no
// agent state file claims. pane_current_command only reports the foreground
// process, so a pane running a shell command from inside the harness can miss
// this path for one refresh and appear on the next idle cycle.
func ReconcileUnregistered(sf *domain.StateFile, targets map[string]domain.PaneTarget, cwds, cmds map[string]string, now time.Time) {
	if sf == nil || len(targets) == 0 || len(cmds) == 0 {
		return
	}
	if sf.Agents == nil {
		sf.Agents = make(map[string]domain.Agent)
	}
	claimed := make(map[string]bool, len(sf.Agents))
	for _, agent := range sf.Agents {
		if agent.TmuxPaneID != "" {
			claimed[agent.TmuxPaneID] = true
		}
	}
	for paneID, target := range targets {
		if claimed[paneID] {
			continue
		}
		harness := harnessFromCommand(cmds[paneID])
		if harness == "" {
			continue
		}
		sessionID := unregisteredID(paneID)
		sf.Agents[sessionID] = domain.Agent{
			Target:     target.Target,
			Session:    target.Session,
			Window:     target.Window,
			Pane:       target.Pane,
			State:      "unregistered",
			Cwd:        cwds[paneID],
			SessionID:  sessionID,
			TmuxPaneID: paneID,
			UpdatedAt:  now.UTC().Format(time.RFC3339),
			Harness:    harness,
		}
	}
}

func harnessFromCommand(cmd string) string {
	switch {
	case cmd == "claude":
		return "claude"
	case strings.HasPrefix(cmd, "codex"):
		return "codex"
	default:
		return ""
	}
}

func unregisteredID(paneID string) string {
	return fmt.Sprintf("%s%s", unregisteredSessionPrefix, paneID)
}

// ReconcileIdentityOptions configures best-effort identity lookup for
// unregistered placeholders.
type ReconcileIdentityOptions struct {
	Store             *Store
	ClaudeProjectsDir string
	ClaudeSessionsDir string
	CodexRoot         string
}

// ReconcileIdentities upgrades placeholders to real harness identities only
// when exact-cwd lookup leaves one safe candidate.
func ReconcileIdentities(sf *domain.StateFile, opts ReconcileIdentityOptions) {
	if sf == nil || len(sf.Agents) == 0 {
		return
	}
	known := knownSessionIDs(sf)
	codexRows, _ := codexthreads.Threads(opts.CodexRoot)
	for key, agent := range sf.Agents {
		if !strings.HasPrefix(agent.SessionID, unregisteredSessionPrefix) {
			continue
		}
		var candidates []identityCandidate
		switch agent.Harness {
		case "codex":
			candidates = codexCandidates(agent.Cwd, codexRows)
		default:
			candidates = claudeCandidates(agent.Cwd, opts.ClaudeSessionsDir, opts.ClaudeProjectsDir)
		}
		candidates = filterIdentityCandidates(candidates, known, opts.Store)
		if len(candidates) != 1 {
			continue
		}
		candidate := candidates[0]
		delete(sf.Agents, key)
		agent.SessionID = candidate.sessionID
		agent.Branch = candidate.branch
		agent.LastMessagePreview = candidate.preview
		agent.State = "unregistered"
		sf.Agents[agent.SessionID] = agent
		known[agent.SessionID] = true
	}
}

type identityCandidate struct {
	sessionID string
	branch    string
	preview   string
	updatedAt string
}

func knownSessionIDs(sf *domain.StateFile) map[string]bool {
	known := make(map[string]bool, len(sf.Agents))
	for _, agent := range sf.Agents {
		if agent.SessionID != "" && !strings.HasPrefix(agent.SessionID, unregisteredSessionPrefix) {
			known[agent.SessionID] = true
		}
	}
	return known
}

func filterIdentityCandidates(candidates []identityCandidate, known map[string]bool, store *Store) []identityCandidate {
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.sessionID == "" || known[candidate.sessionID] || store.Dismissed(candidate.sessionID) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func codexCandidates(cwd string, rows []codexthreads.Thread) []identityCandidate {
	var out []identityCandidate
	for _, row := range rows {
		if row.Archived || row.Cwd != cwd {
			continue
		}
		out = append(out, identityCandidate{
			sessionID: row.ID,
			branch:    row.GitBranch,
			preview:   row.FirstUserMessage,
			updatedAt: row.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].updatedAt > out[j].updatedAt
	})
	return out
}

type claudeSessionFile struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
}

func claudeCandidates(cwd, sessionsDir, projectsDir string) []identityCandidate {
	if cwd == "" || sessionsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}
	var sessions []claudeSessionFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name()))
		if err != nil {
			continue
		}
		var sf claudeSessionFile
		if json.Unmarshal(data, &sf) != nil || filepath.Clean(sf.Cwd) != filepath.Clean(cwd) {
			continue
		}
		sessions = append(sessions, sf)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt > sessions[j].StartedAt
	})

	candidates := make([]identityCandidate, 0, len(sessions))
	for _, sf := range sessions {
		projDir := conversation.PickProjDir(projectsDir, sf.SessionID, cwd)
		preview := ""
		if entries := conversation.ReadConversation(projDir, sf.SessionID, 1); len(entries) > 0 {
			preview = entries[0].Content
		}
		candidates = append(candidates, identityCandidate{
			sessionID: sf.SessionID,
			branch:    conversation.LastGitBranch(projDir, sf.SessionID),
			preview:   preview,
			updatedAt: fmt.Sprintf("%020d", sf.StartedAt),
		})
	}
	return candidates
}
