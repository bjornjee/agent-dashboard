package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

	// Build the shell command — passed directly to new-window as the
	// pane's initial process to avoid tmux send-keys buffer limits.
	cmd := s.cfg.Profile.Command
	prompt := buildPrompt(req.Skill, req.Message)
	if prompt != "" {
		cmd = cmd + " " + shellQuote(prompt)
	}

	target, err := tmux.TmuxNewWindow(session, repoName, folder, cmd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create window: %v", err)})
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

// handlePRURL resolves and returns the PR URL for an agent.
// If the agent already has a pr_url, it appends /files and returns it.
// Otherwise it tries `gh pr view` to find an existing PR, falling back
// to a GitHub compare URL constructed from the remote and branch.
func (s *Server) handlePRURL(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	// If pr_url is already stored, use it directly.
	if agent.PRURL != "" {
		prURL := strings.TrimRight(agent.PRURL, "/") + "/files"
		writeJSON(w, http.StatusOK, map[string]string{"url": prURL})
		return
	}

	dir := agent.EffectiveDir()
	branch := agent.Branch
	if dir == "" || branch == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no directory or branch"})
		return
	}

	// Try gh pr view to find an existing PR.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	out, err := cmdRunner.CombinedOutput(ctx, dir, "gh", "pr", "view", branch,
		"--json", "url", "-q", ".url")
	if err == nil {
		if prURL := strings.TrimSpace(string(out)); prURL != "" {
			writeJSON(w, http.StatusOK, map[string]string{"url": strings.TrimRight(prURL, "/") + "/files"})
			return
		}
	}

	// Fall back to compare URL.
	prURL, err := buildCompareURL(ctx, dir, branch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": prURL})
}

// buildCompareURL constructs a GitHub compare URL from the repo remote and branch.
func buildCompareURL(ctx context.Context, dir, branch string) (string, error) {
	out, err := cmdRunner.CombinedOutput(ctx, dir, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("failed to get remote: %w", err)
	}
	remoteURL := strings.TrimSpace(string(out))

	owner, repo, ok := parseGitHubRemote(remoteURL)
	if !ok {
		return "", fmt.Errorf("not a GitHub remote: %s", remoteURL)
	}

	base := gitDefaultBranchFromDir(ctx, dir)
	return fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s?expand=1",
		url.PathEscape(owner),
		url.PathEscape(repo),
		url.PathEscape(base),
		url.PathEscape(branch),
	), nil
}

// parseGitHubRemote extracts owner and repo from a GitHub remote URL.
func parseGitHubRemote(remoteURL string) (owner, repo string, ok bool) {
	remoteURL = strings.TrimSpace(remoteURL)

	// SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path := strings.TrimPrefix(remoteURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}

	// HTTPS: https://github.com/owner/repo.git
	if strings.HasPrefix(remoteURL, "https://github.com/") {
		path := strings.TrimPrefix(remoteURL, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}

	return "", "", false
}

// gitDefaultBranchFromDir returns the default branch for the repo in dir.
func gitDefaultBranchFromDir(ctx context.Context, dir string) string {
	out, err := cmdRunner.CombinedOutput(ctx, dir, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		if parts := strings.Split(ref, "/"); len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return "main"
}
