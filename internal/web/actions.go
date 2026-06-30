package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/diagrams"
	"github.com/bjornjee/agent-dashboard/internal/gh"
	"github.com/bjornjee/agent-dashboard/internal/harness"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/repowin"
	"github.com/bjornjee/agent-dashboard/internal/skills"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// handleApprove sends a plan / permission approval to an agent's tmux
// pane. Claude uses the single-letter "y" shortcut at its picker. Codex
// has no equivalent picker, so the approval is sent as a literal chat
// message via the same paste-buffer + bracketed-paste path used by
// handleInput — the agent then reads the message and continues.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.sendApprovalKeystroke(w, r, "y", "Approve", "approved")
}

// handleReject sends a plan / permission rejection. Same harness split
// as handleApprove — claude takes "n", codex takes a literal explanatory
// message so the agent revises its plan.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	s.sendApprovalKeystroke(w, r, "n", "Reject — please revise the plan", "rejected")
}

func (s *Server) sendApprovalKeystroke(w http.ResponseWriter, r *http.Request, claudeKey, codexText, okMessage string) {
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
	var err error
	if agent.Harness == "codex" {
		err = tmux.TmuxPasteKeysClearingInput(target, codexText)
		if err == nil {
			err = tmux.TmuxSendRawKeys(target, codex.SubmitKeysAfterPaste(target, agent.State)...)
		}
	} else {
		err = tmux.TmuxSendKeys(target, claudeKey)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": okMessage})
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
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text required"})
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
	// Codex needs paste-buffer + bracketed-paste delivery. Prefer the
	// visible pane mode over the sidecar state: hooks can leave state as
	// "running" after the prompt is already idle, while the Codex footer
	// explicitly says when Tab is required to queue a reply.
	var sendErr error
	if agent.Harness == "codex" {
		sendErr = tmux.TmuxPasteKeysClearingInput(target, req.Text)
		if sendErr == nil {
			sendErr = tmux.TmuxSendRawKeys(target, codex.SubmitKeysAfterPaste(target, agent.State)...)
		}
	} else {
		sendErr = tmux.TmuxSendKeys(target, req.Text)
	}
	if sendErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sendErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "sent"})
}

// askAnswerEntry is one question's answer in an /answer-question payload.
// option_indices are 0-based positions into the original AskUserQuestion
// options array. freeform is non-empty when the user picked the "Other" /
// Type-something path. multi mirrors the question's multi_select flag and
// tells the handler whether to append Tab after the option-number keys
// (single-select auto-advances; multi-select needs an explicit advance).
type askAnswerEntry struct {
	OptionIndices []int  `json:"option_indices"`
	Freeform      string `json:"freeform"`
	Multi         bool   `json:"multi"`
}

// askAnswerRequest is the JSON body for /answer-question.
// answers[i] is the answer for the i-th question in the pending payload;
// option_counts[i] is that question's original options.length, used to
// compute the "Other" digit (= option_counts[i] + 1).
type askAnswerRequest struct {
	Answers      []askAnswerEntry `json:"answers"`
	OptionCounts []int            `json:"option_counts"`
}

// handleAnswerQuestion drives the harness-appropriate question picker
// via a translated key sequence.
//
// Claude Code's AskUserQuestion uses digit shortcuts (k picks/toggles
// option k; Other lives at option_count+1; final Enter commits Submit).
//
// Codex's request_user_input has no digit shortcuts — its footer is
// "tab to add notes | enter to submit". The driver navigates with Down
// arrows and commits per-question with Enter; codex advances on Enter
// so no final Submit-tab is needed.
func (s *Server) handleAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	wantTool := "AskUserQuestion"
	if agent.Harness == "codex" {
		wantTool = "request_user_input"
	}
	// Single paused-on-tool predicate shared with handlePendingQuestion.
	// Driving the picker is safe iff one of:
	//   1. JSONL/rollout fallback says unanswered — authoritative; the
	//      CLI is blocked at the picker (covers the codex Stop race
	//      where the hook missed PreToolUse).
	//   2. Sidecar PendingQuestion is set AND CurrentTool matches —
	//      the hook just stamped both and hasn't cleared them.
	// Sidecar set + CurrentTool mismatch means a stale snapshot; reject
	// to avoid sending picker keys into whatever buffer the CLI is
	// actually showing.
	roots := conversation.Roots{CodexSessionsRoot: s.codexSessionsRootDir}
	pq := PausedOnQuestion(agent, roots)
	if pq == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent is not paused on " + wantTool})
		return
	}
	sidecarStale := agent.PendingQuestion != nil && agent.CurrentTool != wantTool
	if sidecarStale {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent is not paused on " + wantTool})
		return
	}
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}

	var req askAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(req.Answers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "answers required"})
		return
	}
	if len(req.Answers) != len(req.OptionCounts) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "answers and option_counts length mismatch"})
		return
	}
	// Validate every answer up-front. Partial keystroke delivery would
	// leave the picker in a half-answered state — refuse before sending.
	for i, ans := range req.Answers {
		optCount := req.OptionCounts[i]
		if optCount <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("answer %d: option_count must be > 0", i)})
			return
		}
		if ans.Freeform == "" && len(ans.OptionIndices) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("answer %d: no option picked and no freeform text", i)})
			return
		}
		for _, idx := range ans.OptionIndices {
			if idx < 0 || idx >= optCount {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("answer %d: option_index %d out of range [0,%d)", i, idx, optCount)})
				return
			}
		}
	}

	target := tmux.ResolveTarget(agent.TmuxPaneID)
	if target == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pane no longer exists"})
		return
	}

	driver := driveAskUserQuestionPicker
	if agent.Harness == "codex" {
		driver = driveCodexRequestUserInputPicker
	}
	if err := driver(target, req); err != nil {
		// Abort the picker so it returns to a clean cancelled state
		// instead of half-answered. Best-effort — if Escape also fails
		// the user can ESC manually from the terminal.
		_ = tmux.TmuxSendRaw(target, "Escape")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "answered"})
}

// driveCodexRequestUserInputPicker translates a validated answer payload
// into the tmux send-keys sequence codex's request_user_input picker
// accepts.
//
// Each question in codex's picker:
//   - The first option is highlighted on entry.
//   - Down arrow moves the highlight; codex shows no digit shortcut.
//   - Enter submits the current question and codex advances to the next.
//   - Tab opens an "add notes" freeform input on the highlighted option;
//     subsequent text + Enter commits the note. The SKILL doc confirms
//     codex auto-appends an "Other"/"None of the above" entry as the
//     last selectable row — freeform answers navigate to it first.
//
// Codex's request_user_input has no multi_select; ans.Multi is ignored.
// The first entry in OptionIndices is the picked option for that
// question. After all questions land, no final Enter is sent — codex
// already advanced past the last question on its own Enter.
func driveCodexRequestUserInputPicker(target string, req askAnswerRequest) error {
	for i, ans := range req.Answers {
		optCount := req.OptionCounts[i]
		if ans.Freeform != "" {
			// Navigate to the auto-added Other entry: it sits one row
			// past the labeled options. From the first row that means
			// optCount Down presses.
			for j := 0; j < optCount; j++ {
				if err := tmux.TmuxSendRaw(target, "Down"); err != nil {
					return err
				}
			}
			if err := tmux.TmuxSendRaw(target, "Tab"); err != nil {
				return err
			}
			// TmuxSendKeys sends the literal text and follows with Enter
			// automatically, so no separate submit keystroke is needed.
			if err := tmux.TmuxSendKeys(target, ans.Freeform); err != nil {
				return err
			}
			continue
		}
		// Single-select: first OptionIndex is the pick. Move Down (idx)
		// times from the default first-row highlight, then Enter.
		idx := 0
		if len(ans.OptionIndices) > 0 {
			idx = ans.OptionIndices[0]
		}
		for j := 0; j < idx; j++ {
			if err := tmux.TmuxSendRaw(target, "Down"); err != nil {
				return err
			}
		}
		if err := tmux.TmuxSendRaw(target, "Enter"); err != nil {
			return err
		}
	}
	return nil
}

// driveAskUserQuestionPicker translates a validated askAnswerRequest into
// the exact tmux send-keys sequence Claude Code's picker expects.
//
// For each answer:
//   - Freeform: press the Other digit (optCount+1) to open the text input,
//     then send the text + Enter via the literal-text path.
//   - Single-select with N picks: press each picked option's digit; the
//     picker auto-advances on each single-select pick.
//   - Multi-select: press each picked option's digit (toggles), then press
//     Tab to advance to the next question.
//
// After all answers, one final Enter commits the Submit tab. Returns the
// first tmux error encountered; the caller is responsible for sending
// Escape to abort the picker on failure.
func driveAskUserQuestionPicker(target string, req askAnswerRequest) error {
	for i, ans := range req.Answers {
		optCount := req.OptionCounts[i]
		switch {
		case ans.Freeform != "":
			otherDigit := strconv.Itoa(optCount + 1)
			if err := tmux.TmuxSendRaw(target, otherDigit); err != nil {
				return err
			}
			if err := tmux.TmuxSendKeys(target, ans.Freeform); err != nil {
				return err
			}
		default:
			for _, idx := range ans.OptionIndices {
				digit := strconv.Itoa(idx + 1)
				if err := tmux.TmuxSendRaw(target, digit); err != nil {
					return err
				}
			}
			if ans.Multi {
				if err := tmux.TmuxSendRaw(target, "Tab"); err != nil {
					return err
				}
			}
		}
	}
	// Commit the Submit tab.
	return tmux.TmuxSendRaw(target, "Enter")
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
	s.clearTrustPane(agent.TmuxPaneID)

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

// handleCleanup performs post-merge cleanup: kills the tmux pane, removes the
// worktree, checks out the default branch, pulls, deletes the local feature
// branch, removes the agent state file, and cleans up diagram data.
func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, ok := s.lookupAgent(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	cwd := agent.Cwd
	worktreeCwd := agent.WorktreeCwd
	branch := agent.Branch

	if cwd == "" || !filepath.IsAbs(cwd) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no valid working directory"})
		return
	}
	if worktreeCwd != "" && !filepath.IsAbs(worktreeCwd) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid worktree path"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Resolve default branch
	defaultBranch := gitDefaultBranchFromDir(ctx, cwd)

	// 1. Kill tmux pane (ignore errors — pane may already be dead)
	if tmux.TmuxIsAvailable() {
		if target := tmux.ResolveTarget(agent.TmuxPaneID); target != "" {
			tmux.TmuxKillPane(target)
		}
	}

	// 2. Remove worktree if applicable
	if worktreeCwd != "" {
		if _, err := cmdRunner.CombinedOutput(ctx, cwd, "git", "-C", cwd, "worktree", "remove", "--force", worktreeCwd); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "worktree remove failed"})
			return
		}
		cmdRunner.CombinedOutput(ctx, cwd, "git", "-C", cwd, "worktree", "prune")
	}

	// 3. Checkout default branch
	if _, err := cmdRunner.CombinedOutput(ctx, cwd, "git", "-C", cwd, "checkout", defaultBranch); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("checkout %s failed", defaultBranch)})
		return
	}

	// 4. Pull default branch
	if _, err := cmdRunner.CombinedOutput(ctx, cwd, "git", "-C", cwd, "pull", "origin", defaultBranch); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("pull origin %s failed", defaultBranch)})
		return
	}

	// 5. Delete local feature branch (ignore errors — may already be gone)
	if branch != "" {
		cmdRunner.CombinedOutput(ctx, cwd, "git", "-C", cwd, "branch", "-d", branch)
	}

	// 6. Remove agent state file
	if err := state.RemoveAgent(s.cfg.Profile.StateDir, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove state file"})
		return
	}

	// 7. Clean up diagram data (best-effort — diagrams are cosmetic)
	_ = diagrams.CleanupSession(s.cfg.Profile.StateDir, id)

	writeJSON(w, http.StatusOK, map[string]string{"ok": "cleaned up"})
}

// createRequest is the JSON body for agent creation.
type createRequest struct {
	Folder  string `json:"folder"`
	Skill   string `json:"skill"`
	Message string `json:"message"`
	// Harness overrides the [harness] default in settings.toml for this
	// spawn only. Empty string falls back to the configured default.
	// Valid: "claude" | "codex".
	Harness string `json:"harness"`
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

	// Resolve the harness for this spawn — request override takes precedence
	// over the configured default. Per-harness settings
	// (Codex.Model/Approval/Sandbox/effort) flow into SpawnOpts based on the
	// active harness; other harnesses ignore the unused fields. Unknown
	// harness names are HTTP 400 (not silently coerced to claude — see
	// harness.Resolve docs).
	activeHarness := s.cfg.Harness
	if req.Harness != "" && req.Harness != activeHarness.Name() {
		h, hErr := harness.Resolve(req.Harness, s.cfg.Profile)
		if hErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": hErr.Error()})
			return
		}
		activeHarness = h
	}
	if !skills.SupportsHarness(req.Skill, activeHarness.Name()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "skill '" + req.Skill + "' is not supported by " + activeHarness.Name(),
		})
		return
	}
	spawnOpts := harness.SpawnOptsFor(activeHarness.Name(), s.cfg.Settings)
	cmd := activeHarness.SpawnCommand(req.Skill, req.Message, spawnOpts)

	target, paneID, ok := s.spawnInRepoWindow(w, folder, cmd)
	if !ok {
		return // spawnInRepoWindow already wrote the error response
	}
	// Stage the worktree/branch pin keyed by the new pane_id so the dashboard
	// renders correctly *before* the agent's first hook event fires.
	_ = state.StageSpawnPin(s.cfg.Profile.StateDir, folder, paneID, target)

	// Mirror the TUI's post-spawn trust-folder detection so the web
	// dashboard surfaces the harness's "trust this folder?" dialog
	// instead of leaving the agent silently stuck. The watcher runs in
	// the background with its own context tied to trustWatchBudget so
	// the HTTP handler returns immediately.
	if paneID != "" {
		go s.watchTrustPrompt(context.Background(), paneID, target, folder, trustWatchBudget, trustWatchTick)
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "created", "target": target})
}

// spawnInRepoWindow launches cmd in a new pane for folder's repo: it splits an
// existing window for that repo when one exists and is under the pane limit,
// otherwise it opens a fresh window. On any failure it writes the HTTP error
// response and returns ok=false. Shared by handleCreate and handleResume so
// both spawn paths place panes identically.
func (s *Server) spawnInRepoWindow(w http.ResponseWriter, folder, cmd string) (target, paneID string, ok bool) {
	rawRepo := repowin.RepoFromCwd(folder)
	if rawRepo == "" {
		rawRepo = s.cfg.Profile.Command
	}
	repoName := repowin.SanitizeWindowName(rawRepo)
	// Look for an existing window with agents in the same repo.
	agents := s.readAgentState()
	sw, found := repowin.FindWindowForRepo(agents, folder, "")
	if !found {
		// Fallback: match by tmux window name in the first session.
		session, sErr := firstTmuxSession()
		if sErr == nil {
			windows, wErr := tmux.TmuxListWindows(session)
			if wErr == nil {
				sw, found = repowin.FindWindowByName(windows, repoName, session, "")
			}
		}
	}

	if found {
		// Split into the existing window if under the pane limit.
		count, cErr := tmux.TmuxCountPanes(sw)
		if cErr != nil {
			found = false // window may have been destroyed; fall through
		} else if count >= repowin.MaxPanesPerWindow {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("%d-pane limit reached for %s", repowin.MaxPanesPerWindow, repoName),
			})
			return "", "", false
		} else {
			var sErr error
			target, paneID, sErr = tmux.TmuxSplitWindow(sw, folder, cmd)
			if sErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("split window: %v", sErr)})
				return "", "", false
			}
		}
	}
	if !found {
		// No existing window — create a new one.
		session, sErr := firstTmuxSession()
		if sErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no tmux sessions available"})
			return "", "", false
		}
		var nErr error
		target, paneID, nErr = tmux.TmuxNewWindow(session, repoName, folder, cmd)
		if nErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create window: %v", nErr)})
			return "", "", false
		}
	}
	return target, paneID, true
}

// handleResume re-spawns a restart-survivor (orphaned) agent on its existing
// session — `claude --resume <sid>` / `codex resume <sid>` — in a fresh pane in
// its stored working directory. It reads the agent via state.ReadAgent (the
// single-file fast path: no worktree/branch/projdir resolve chain, since the
// persisted record already holds session_id, harness, and cwd/worktree_cwd).
// On success the stale orphan state file is removed: claude reuses the same sid
// so its SessionStart hook re-creates a live file; codex resume gets a new sid,
// so removing the old file prevents a duplicate dead row.
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if !tmux.TmuxIsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tmux not available"})
		return
	}
	id := r.PathValue("id")
	agent, found := state.ReadAgent(s.cfg.Profile.StateDir, id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	if agent.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no session to resume"})
		return
	}

	folder := agent.EffectiveDir()
	if folder == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no working directory"})
		return
	}
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent working directory no longer exists"})
		return
	}

	// Only resume a genuine orphan (its pane is dead). Resuming a live agent
	// would spawn a duplicate session and delete the live agent's state file.
	livePaneIDs := tmux.TmuxListLivePaneIDs()
	if livePaneIDs == nil {
		// tmux enumeration failed — we can't tell live from dead. Refuse rather
		// than risk resuming a live agent; the caller can retry.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not enumerate tmux panes; try again"})
		return
	}
	if !state.IsResumableOrphan(agent, livePaneIDs) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent is not a resumable orphan (its pane may still be alive)"})
		return
	}

	// Empty agent.Harness routes to claude (legacy pre-codex state files).
	h, hErr := harness.Resolve(agent.Harness, s.cfg.Profile)
	if hErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": hErr.Error()})
		return
	}
	opts := harness.SpawnOptsFor(h.Name(), s.cfg.Settings)
	opts.ResumeSessionID = agent.SessionID
	cmd := h.SpawnCommand("", "", opts)

	target, paneID, ok := s.spawnInRepoWindow(w, folder, cmd)
	if !ok {
		return // spawnInRepoWindow already wrote the error response
	}

	_ = state.StageSpawnPin(s.cfg.Profile.StateDir, folder, paneID, target)
	// Drop the stale orphan entry; the resumed agent re-registers live.
	_ = state.RemoveAgent(s.cfg.Profile.StateDir, agent.SessionID)

	if paneID != "" {
		go s.watchTrustPrompt(context.Background(), paneID, target, folder, trustWatchBudget, trustWatchTick)
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "resumed", "target": target})
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
		if !isGitHubURL(prURL) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stored PR URL is not a GitHub URL"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": prURL})
		return
	}

	dir := agent.EffectiveDir()
	if dir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no directory"})
		return
	}

	// Detect branch from git if not stored in agent state.
	branch := agent.Branch
	if branch == "" {
		brCtx, brCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer brCancel()
		out, err := cmdRunner.CombinedOutput(brCtx, dir, "git", "branch", "--show-current")
		if err != nil || strings.TrimSpace(string(out)) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not determine branch"})
			return
		}
		branch = strings.TrimSpace(string(out))
	}

	// Try gh pr view to find an existing PR.
	ghCtx, ghCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer ghCancel()
	out, err := cmdRunner.CombinedOutput(ghCtx, dir, "gh", "pr", "view", branch,
		"--json", "url", "-q", ".url")
	if err == nil {
		if prURL := strings.TrimSpace(string(out)); prURL != "" && isGitHubURL(prURL) {
			writeJSON(w, http.StatusOK, map[string]string{"url": strings.TrimRight(prURL, "/") + "/files"})
			return
		}
	}

	// Fall back to compare URL with a fresh context budget.
	fallbackCtx, fallbackCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer fallbackCancel()
	prURL, err := buildCompareURL(fallbackCtx, dir, branch)
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

// isGitHubURL validates that a URL points to github.com over HTTPS.
func isGitHubURL(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "github.com"
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
