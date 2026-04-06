package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/gh"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// handleApprove sends approval to an agent's tmux pane.
// For permission state: sends "y". For plan state: sends "1".
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}
	target := tmux.ResolveTarget(agent.TmuxPaneID)
	if target == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pane no longer exists"})
		return
	}

	key := "y"
	if agent.EffectiveState() == "plan" {
		key = "1"
	}
	if err := tmux.TmuxSendKeys(target, key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "approved"})
}

// handleReject sends rejection ("n") to an agent's tmux pane.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}
	target := tmux.ResolveTarget(agent.TmuxPaneID)
	if target == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pane no longer exists"})
		return
	}
	if err := tmux.TmuxSendKeys(target, "n"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "rejected"})
}

// inputRequest is the JSON body for the input endpoint.
type inputRequest struct {
	Text string `json:"text"`
}

// handleInput sends text input to an agent's tmux pane.
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}

	var req inputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(req.Text) > 4096 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input too long (max 4096 chars)"})
		return
	}

	target := tmux.ResolveTarget(agent.TmuxPaneID)
	if target == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pane no longer exists"})
		return
	}
	if err := tmux.TmuxSendKeys(target, req.Text); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "sent"})
}

// handleStop sends Ctrl+C to an agent's tmux pane.
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}
	target := tmux.ResolveTarget(agent.TmuxPaneID)
	if target == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pane no longer exists"})
		return
	}
	if err := tmux.TmuxSendRaw(target, "C-c"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "stopped"})
}

// handleClose kills an agent's tmux pane and removes its state file.
func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, ok := s.lookupAgent(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	// Kill tmux pane if available
	if tmux.TmuxIsAvailable() {
		target := tmux.ResolveTarget(agent.TmuxPaneID)
		if target != "" {
			tmux.TmuxKillPane(target)
		}
	}

	// Remove state file
	if err := state.RemoveAgent(s.cfg.Profile.StateDir, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "closed"})
}

// handleMerge runs `gh pr merge --squash` for the agent's branch.
// If the authenticated user is a CODEOWNERS entry, --admin is appended
// to bypass branch protection rules.
func (s *Server) handleMerge(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	dir := agent.EffectiveDir()
	if dir == "" || agent.Branch == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no directory or branch"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	args := gh.MergeArgs(cmdRunner, dir, agent.Branch)
	out, err := cmdRunner.CombinedOutput(ctx, dir, "gh", args...)
	if err != nil {
		detail := strings.TrimSpace(string(out))
		msg := "gh pr merge failed"
		if detail != "" {
			msg = fmt.Sprintf("gh pr merge: %s", detail)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
		return
	}

	// Pin state to "merged"
	state.PinAgentState(s.cfg.Profile.StateDir, r.PathValue("id"), "merged")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "merged"})
}

// createRequest is the JSON body for agent creation.
type createRequest struct {
	Folder  string `json:"folder"`
	Skill   string `json:"skill"`
	Message string `json:"message"`
}

// handleCreate spawns a new agent session in a tmux pane.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Folder == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "folder is required"})
		return
	}

	// Expand ~ in folder path
	folder := req.Folder
	if strings.HasPrefix(folder, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve home directory"})
			return
		}
		folder = filepath.Join(home, folder[2:])
	}

	// Validate folder exists and is a directory
	fi, err := os.Stat(folder)
	if err != nil || !fi.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "folder does not exist or is not a directory"})
		return
	}

	// Create a new tmux window for the agent
	repoName := repoFromPath(folder)
	if repoName == "" {
		repoName = s.cfg.Profile.Command
	}

	// Find a tmux session to create the window in.
	// Use the first available session from tmux list-sessions.
	session, err := firstTmuxSession()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no tmux sessions available"})
		return
	}

	target, err := tmux.TmuxNewWindow(session, repoName, folder)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create window: %v", err)})
		return
	}

	// Build command with optional skill and message
	cmd := s.cfg.Profile.Command
	prompt := buildPrompt(req.Skill, req.Message)
	if prompt != "" {
		cmd = cmd + " " + shellQuote(prompt)
	}

	if err := tmux.TmuxSendKeys(target, cmd); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("send command: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "created", "target": target})
}

func buildPrompt(skill, message string) string {
	var parts []string
	if skill != "" {
		parts = append(parts, "/"+skill)
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func repoFromPath(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// firstTmuxSession returns the name of the first available tmux session.
func firstTmuxSession() (string, error) {
	sessions, err := tmux.TmuxListSessions()
	if err != nil {
		return "", err
	}
	return sessions[0], nil
}
