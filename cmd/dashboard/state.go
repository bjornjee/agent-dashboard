package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Agent represents a single Claude Code agent's state.
type Agent struct {
	Target             string   `json:"target"`
	Session            string   `json:"session"`
	Window             int      `json:"window"`
	Pane               int      `json:"pane"`
	State              string   `json:"state"`
	Cwd                string   `json:"cwd"`
	Branch             string   `json:"branch"`
	SessionID          string   `json:"session_id"`
	StartedAt          string   `json:"started_at"`
	UpdatedAt          string   `json:"updated_at"`
	LastMessagePreview string   `json:"last_message_preview"`
	FilesChanged       []string `json:"files_changed"`
	Model              string   `json:"model"`
	PermissionMode     string   `json:"permission_mode"`
	SubagentCount      int      `json:"subagent_count"`
	LastHookEvent      string   `json:"last_hook_event"`
	CurrentTool        string   `json:"current_tool"`
}

// StateFile is the top-level JSON structure.
type StateFile struct {
	Agents map[string]Agent `json:"agents"`
}

// State groups: needs attention → running → completed
var statePriority = map[string]int{
	"input":   1, // needs attention
	"error":   1, // needs attention
	"running": 2,
	"idle":    3, // completed
	"done":    3, // completed
}

// DefaultStatePath returns ~/.claude/agent-dashboard/state.json.
func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/agent-dashboard/state.json"
	}
	return filepath.Join(home, ".claude", "agent-dashboard", "state.json")
}

// ReadState reads and parses the state file. Returns empty state on any error.
func ReadState(path string) StateFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return StateFile{Agents: make(map[string]Agent)}
	}

	var sf StateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return StateFile{Agents: make(map[string]Agent)}
	}

	if sf.Agents == nil {
		sf.Agents = make(map[string]Agent)
	}
	return sf
}

// SortedAgents returns agents sorted by state priority, then by updated_at.
// selfTarget is excluded from the list (the dashboard's own pane).
func SortedAgents(sf StateFile, selfTarget string) []Agent {
	agents := make([]Agent, 0, len(sf.Agents))
	for _, a := range sf.Agents {
		if a.Target == "" || a.State == "" {
			continue
		}
		if ValidateTarget(a.Target) != nil {
			continue
		}
		if selfTarget != "" && a.Target == selfTarget {
			continue
		}
		agents = append(agents, a)
	}

	sort.Slice(agents, func(i, j int) bool {
		pi := statePriority[agents[i].State]
		pj := statePriority[agents[j].State]
		if pi == 0 {
			pi = 99
		}
		if pj == 0 {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		// Stable sort by window, then pane within same priority group
		if agents[i].Window != agents[j].Window {
			return agents[i].Window < agents[j].Window
		}
		return agents[i].Pane < agents[j].Pane
	})

	return agents
}

// CleanStale removes agents that haven't been updated within maxAgeSecs.
func CleanStale(path string, maxAgeSecs int) {
	sf := ReadState(path)
	now := time.Now()
	changed := false

	for id, agent := range sf.Agents {
		t, err := time.Parse(time.RFC3339, agent.UpdatedAt)
		if err != nil || now.Sub(t).Seconds() > float64(maxAgeSecs) {
			delete(sf.Agents, id)
			changed = true
		}
	}

	if changed {
		data, _ := json.Marshal(sf)
		_ = os.WriteFile(path, data, 0644)
	}
}

// PruneDead removes agents whose tmux panes no longer exist.
// renames maps oldTarget → newTarget for panes that were renumbered
// (e.g., due to tmux renumber-windows). Renamed agents are updated
// in-place rather than deleted. Pass nil if no renames are known.
// Returns the number of agents removed.
func PruneDead(path string, livePanes map[string]bool, renames map[string]string) int {
	sf := ReadState(path)
	changed := false
	removed := 0

	// First pass: apply renames for agents whose targets changed
	for oldTarget, newTarget := range renames {
		agent, exists := sf.Agents[oldTarget]
		if !exists {
			continue
		}
		delete(sf.Agents, oldTarget)
		agent.Target = newTarget
		sf.Agents[newTarget] = agent
		changed = true
	}

	// Second pass: remove truly dead agents
	for id := range sf.Agents {
		if !livePanes[id] {
			delete(sf.Agents, id)
			removed++
		}
	}

	if removed > 0 || changed {
		data, _ := json.Marshal(sf)
		_ = os.WriteFile(path, data, 0644)
	}
	return removed
}

// RemoveAgent removes an agent from the state file by target.
func RemoveAgent(path, target string) error {
	sf := ReadState(path)
	delete(sf.Agents, target)
	data, err := json.Marshal(sf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// FormatDuration returns a human-readable duration since the given ISO8601 timestamp.
func FormatDuration(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return ""
	}

	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, secs%60)
	}
	hours := mins / 60
	return fmt.Sprintf("%dh %dm", hours, mins%60)
}
