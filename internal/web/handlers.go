package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/usage"
)

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
	Path   string `json:"path"`
	Status string `json:"status"` // "added", "modified", "deleted"
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
	cmd := exec.Command("git", "diff", "--no-color")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusOK, diffResponse{})
		return
	}

	// Parse file list from diff
	var files []diffFile
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				path := strings.TrimPrefix(parts[3], "b/")
				files = append(files, diffFile{Path: path, Status: "modified"})
			}
		}
	}

	writeJSON(w, http.StatusOK, diffResponse{
		Raw:   string(out),
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
