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

// DefaultStateDir returns ~/.claude/agent-dashboard.
func DefaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/agent-dashboard"
	}
	return filepath.Join(home, ".claude", "agent-dashboard")
}

// agentsDir returns the agents subdirectory within the state directory.
func agentsDir(dir string) string {
	return filepath.Join(dir, "agents")
}

// ReadState reads all per-agent JSON files from dir/agents/*.json.
// Returns empty state on any error.
func ReadState(dir string) StateFile {
	sf := StateFile{Agents: make(map[string]Agent)}

	entries, err := os.ReadDir(agentsDir(dir))
	if err != nil {
		return sf
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentsDir(dir), entry.Name()))
		if err != nil {
			continue
		}
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		if agent.Target != "" {
			sf.Agents[agent.Target] = agent
		}
	}

	return sf
}

// agentFileMap returns a map of target → file path for all agent files.
func agentFileMap(dir string) map[string]string {
	m := make(map[string]string)
	entries, err := os.ReadDir(agentsDir(dir))
	if err != nil {
		return m
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(agentsDir(dir), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		if agent.Target != "" {
			m[agent.Target] = path
		}
	}
	return m
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

// CleanStale removes agent files that haven't been updated within maxAgeSecs.
func CleanStale(dir string, maxAgeSecs int) {
	now := time.Now()
	entries, err := os.ReadDir(agentsDir(dir))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(agentsDir(dir), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil || agent.UpdatedAt == "" {
			_ = os.Remove(path)
			continue
		}
		t, err := time.Parse(time.RFC3339, agent.UpdatedAt)
		if err != nil || now.Sub(t).Seconds() > float64(maxAgeSecs) {
			_ = os.Remove(path)
		}
	}
}

// PruneDead removes agent files whose tmux panes no longer exist.
// renames maps oldTarget → newTarget for panes that were renumbered
// (e.g., due to tmux renumber-windows). Renamed agents are updated
// in-place rather than deleted. Pass nil if no renames are known.
// Returns the number of agents removed.
func PruneDead(dir string, livePanes map[string]bool, renames map[string]string) int {
	files := agentFileMap(dir)
	totalAgents := len(files)
	removed := 0

	// First pass: apply renames to files on disk. Each rename is looked up
	// in the original (unmutated) file map to avoid order-dependent collisions
	// when rename chains overlap (e.g. A→B and B→C).
	for oldTarget, newTarget := range renames {
		path, exists := files[oldTarget]
		if !exists {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		agent.Target = newTarget
		newData, err := json.MarshalIndent(agent, "", "  ")
		if err != nil {
			continue
		}
		_ = os.WriteFile(path, newData, 0644)
	}

	// Re-read the file map after renames to get the accurate post-rename state.
	files = agentFileMap(dir)

	// Second pass: find truly dead agents
	var deadTargets []string
	for target := range files {
		if !livePanes[target] {
			deadTargets = append(deadTargets, target)
		}
	}

	// Safety net: refuse to wipe all agents at once — almost certainly
	// a transient tmux issue. CleanStale handles truly dead agents.
	if len(deadTargets) == totalAgents && totalAgents > 0 {
		return 0
	}

	for _, target := range deadTargets {
		if path, ok := files[target]; ok {
			_ = os.Remove(path)
			removed++
		}
	}

	return removed
}

// RemoveAgent removes an agent's file by target.
func RemoveAgent(dir, target string) error {
	files := agentFileMap(dir)
	path, ok := files[target]
	if !ok {
		return nil // not found, nothing to do
	}
	return os.Remove(path)
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
