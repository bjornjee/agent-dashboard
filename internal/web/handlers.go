package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/usage"
	"github.com/bjornjee/agent-dashboard/internal/zsuggest"
)

// handleSuggestions returns frecency-ranked directory suggestions from ~/.z
// or Claude Code session history. Returns all valid directories (not capped
// at 5 like the TUI) since the browser's datalist filters client-side.
func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	entries := zsuggest.LoadZEntriesWithHome(s.cfg.Profile.HomeDir, s.cfg.Profile.SessionsDir)
	paths := zsuggest.RankAll(entries, zsuggest.DirExists)
	if paths == nil {
		paths = []string{}
	}
	writeJSON(w, http.StatusOK, paths)
}

// CommandRunner abstracts subprocess execution for git/gh commands
// so tests can swap in a mock.
type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	CombinedOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (r *execCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func (r *execCommandRunner) CombinedOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// cmdRunner is the package-level runner used by handlers and actions.
// Tests replace this with a mock.
var cmdRunner CommandRunner = &execCommandRunner{}

// handleConversation returns conversation entries for an agent.
func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(s.cfg.Profile.ProjectsDir, slug)
	entries := conversation.ReadConversation(projDir, agent.SessionID, 100)
	if entries == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleActivity returns the activity log for an agent or subagent.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(s.cfg.Profile.ProjectsDir, slug)
	jsonlPath := filepath.Join(projDir, agent.SessionID+".jsonl")
	entries := conversation.ReadActivityLog(jsonlPath, 500)
	if entries == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// diffResponse holds the structured diff output.
type diffResponse struct {
	Raw   string     `json:"raw"`
	Files []diffFile `json:"files"`
}

type diffFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // "added", "modified", "deleted"
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// findMergeBase returns the merge-base commit between HEAD and main/master,
// or "HEAD" as a fallback (which shows only uncommitted changes).
func findMergeBase(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		out, err := cmdRunner.Output(ctx, "git", "-C", dir, "merge-base", "HEAD", base)
		if err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s
			}
		}
	}
	return "HEAD"
}

// handleDiff returns the git diff for an agent's working directory.
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	dir := agent.EffectiveDir()
	if dir == "" {
		writeJSON(w, http.StatusOK, diffResponse{})
		return
	}

	base := findMergeBase(dir)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := cmdRunner.Output(ctx, "git", "-C", dir, "diff", base, "--no-color")
	if err != nil {
		writeJSON(w, http.StatusOK, diffResponse{})
		return
	}

	// Include untracked files so new files appear in the diff.
	untrackedOut, err := cmdRunner.Output(ctx, "git", "-C", dir,
		"ls-files", "--others", "--exclude-standard")
	if err == nil && len(strings.TrimSpace(string(untrackedOut))) > 0 {
		for _, name := range strings.Split(strings.TrimSpace(string(untrackedOut)), "\n") {
			if name == "" {
				continue
			}
			patch, _ := cmdRunner.Output(ctx, "git", "-C", dir,
				"diff", "--no-index", "--", "/dev/null", name)
			if len(patch) > 0 {
				out = append(out, patch...)
			}
		}
	}

	// Build set of gitignored paths to filter out
	ignored := make(map[string]bool)
	if ignOut, err := cmdRunner.Output(ctx, "git", "-C", dir,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory"); err == nil {
		for _, p := range strings.Split(strings.TrimSpace(string(ignOut)), "\n") {
			p = strings.TrimSuffix(strings.TrimSpace(p), "/")
			if p != "" {
				ignored[p] = true
			}
		}
	}

	// Parse file list from diff with accurate status and line counts
	var files []diffFile
	lines := strings.Split(string(out), "\n")
	// Also rebuild raw diff excluding ignored files
	var filteredLines []string
	skipUntilNext := false
	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			path := ""
			if len(parts) >= 4 {
				path = strings.TrimPrefix(parts[3], "b/")
			}

			// Check if this file is gitignored
			isIgnored := false
			if path != "" {
				for ig := range ignored {
					if path == ig || strings.HasPrefix(path, ig+"/") {
						isIgnored = true
						break
					}
				}
			}
			skipUntilNext = isIgnored
			if isIgnored {
				continue
			}

			status := "modified"
			// Scan lines after diff header for status indicators
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				if strings.HasPrefix(lines[j], "new file mode") {
					status = "added"
					break
				}
				if strings.HasPrefix(lines[j], "deleted file mode") {
					status = "deleted"
					break
				}
				if strings.HasPrefix(lines[j], "diff --git") {
					break
				}
			}
			// Count additions and deletions for this file
			adds, dels := 0, 0
			for j := i + 1; j < len(lines); j++ {
				if j > i && strings.HasPrefix(lines[j], "diff --git") {
					break
				}
				if strings.HasPrefix(lines[j], "+") && !strings.HasPrefix(lines[j], "+++") {
					adds++
				} else if strings.HasPrefix(lines[j], "-") && !strings.HasPrefix(lines[j], "---") {
					dels++
				}
			}
			files = append(files, diffFile{Path: path, Status: status, Additions: adds, Deletions: dels})
		}
		if !skipUntilNext {
			filteredLines = append(filteredLines, line)
		}
	}

	// Use filtered output
	filteredRaw := strings.Join(filteredLines, "\n")

	writeJSON(w, http.StatusOK, diffResponse{
		Raw:   filteredRaw,
		Files: files,
	})
}

// handlePlan returns the plan markdown for an agent.
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(s.cfg.Profile.ProjectsDir, slug)
	planSlug := conversation.ReadPlanSlug(projDir, agent.SessionID)
	if planSlug == "" {
		writeJSON(w, http.StatusOK, map[string]string{"content": ""})
		return
	}
	content := conversation.ReadPlanContent(s.cfg.Profile.PlansDir, planSlug)
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// handleUsage returns token usage and cost for a single agent session.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(s.cfg.Profile.ProjectsDir, slug)
	u := usage.ReadUsage(projDir, agent.SessionID)
	writeJSON(w, http.StatusOK, u)
}

// dailyUsageResponse holds the daily cost breakdown.
type dailyUsageResponse struct {
	Days      []dayEntry `json:"days"`
	TotalCost float64    `json:"total_cost"`
	TodayCost float64    `json:"today_cost"`
}

type dayEntry struct {
	Date    string  `json:"date"`
	CostUSD float64 `json:"cost_usd"`
}

// handleDailyUsage returns the daily cost breakdown.
func (s *Server) handleDailyUsage(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusOK, dailyUsageResponse{})
		return
	}
	since := time.Now().AddDate(0, 0, -7)
	days := s.db.CostByDay(since)
	today := time.Now().Format("2006-01-02")

	var entries []dayEntry
	var todayCost float64
	for _, d := range days {
		entries = append(entries, dayEntry{Date: d.Date, CostUSD: d.CostUSD})
		if d.Date == today {
			todayCost = d.CostUSD
		}
	}
	if entries == nil {
		entries = make([]dayEntry, 0)
	}

	writeJSON(w, http.StatusOK, dailyUsageResponse{
		Days:      entries,
		TotalCost: s.db.TotalCost(),
		TodayCost: todayCost,
	})
}

// handleSubagents returns the subagent tree for an agent.
func (s *Server) handleSubagents(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(s.cfg.Profile.ProjectsDir, slug)
	subs := conversation.FindSubagents(projDir, agent.SessionID)
	if subs == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

// handleSSE streams agent state updates via Server-Sent Events.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.register()
	defer s.hub.unregister(ch)

	// Send initial state immediately
	initial := s.readAgentState()
	data, _ := json.Marshal(initial)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte("data: "))
			w.Write(msg)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
