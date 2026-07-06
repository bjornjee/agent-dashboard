package tui

import (
	"time"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/diagrams"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/usage"
)

// -- Messages --

// stateUpdatedMsg carries the agent list resolved entirely off the
// bubbletea main goroutine. loadState and WatchStateDir do the full
// resolve chain (ReadState → ResolveAgent* → ApplyStateArbitration →
// SortedAgents → TopLevelAgents) inside their own
// goroutines, so the Update handler does not block on any filesystem
// walk — including the codex sessions tree walk that previously froze
// keystrokes whenever the per-session cache expired.
type stateUpdatedMsg struct {
	// agents arrive with Resumable already flagged (resolveAgents runs
	// state.FlagResumable against the same tmux enumeration that produced
	// the targets), so the handler never re-derives orphan status.
	agents []domain.Agent
}
type tickMsg time.Time
type jumpResultMsg struct{ err error }
type sendResultMsg struct{ err error }
type captureResultMsg struct{ lines []string }
type conversationMsg struct {
	entries    []domain.ConversationEntry
	fileOffset int64  // byte offset after reading JSONL
	sessionKey string // projDir+sessionID for cache invalidation
}
type pruneDeadMsg struct{ removed int }
type usageMsg struct {
	perAgent map[string]domain.Usage
	total    domain.Usage
}
type persistResultMsg struct{ err error }
type dbDailyUsageMsg struct {
	total     float64
	todayCost float64
	days      []db.DayUsage
}
type activityMsg struct{ entries []domain.ActivityEntry }

// subagentsBatchMsg carries the full agent→subagents map produced by one
// shared sessions-index lookup. Used by the periodic refresh to avoid
// fanning out N independent tea.Cmds per tick.
type subagentsBatchMsg struct {
	byTarget map[string][]domain.SubagentInfo
}
type planMsg struct{ content string }
type diagramsLoadedMsg struct {
	sessionID string
	list      []diagrams.Diagram
}
type diagramOpenedMsg struct{ err error }
type openEditorMsg struct{ err error }
type openWorktreeMsg struct {
	err error
	dir string
}
type openPRMsg struct {
	err   error
	hasPR bool // true when an existing PR was found (vs compare URL)
}
type mergePRMsg struct{ err error }
type postMergeCleanupMsg struct {
	err      error
	progress string // last step name, for error reporting
}
type ghAvailableMsg struct{ available bool }
type depsReadyMsg struct{ deps []depStatus }
type startupMsg struct {
	tmuxAvailable bool
	selfPaneID    string
}
type quoteMsg struct {
	text   string
	author string
}
type pinStateMsg struct{ err error }
type rawKeySentMsg struct {
	err   error
	label string // human-readable ack, e.g. "Plan approved"
}
type selectPaneMsg struct{ err error }
type closeResultMsg struct {
	err error
	// dismissed is the number of resumable orphan state files removed by a
	// dismiss-all; 0 means a regular pane close.
	dismissed int
}
type createSessionMsg struct {
	target string
	err    error
}
type spawningCaptureMsg struct {
	lines  []string
	target string
}
type rateLimitMsg struct {
	rateLimit *domain.RateLimit
}
type codexUsageMsg struct {
	days []usage.CodexDayUsage
}
type codexPersistMsg struct{}
type filesChangedMsg struct {
	target string
	files  []string
}
type codexDBUsageMsg struct {
	days      []db.DayUsage
	totalCost float64
	todayCost float64
}

// -- Modes --

const (
	modeNormal = iota
	modeReply
	modeUsage
	modeConfirmClose
	modeConfirmDismissAll // confirm before dismissing every resumable orphan
	modeConfirmDeleteDiagram
	modeConfirmMerge   // confirm before merging a PR
	modeConfirmCleanup // confirm before post-merge cleanup
	modeConfirmSend    // confirm before sending a key to an agent pane
	modeConfirmJump    // confirm before jumping to an agent pane
	modeCreateFolder
	modeCreateHarness // harness selection step of create wizard
	modeCreateSkill   // skill selection step of create wizard
	modeCreateModel   // model selection step of create wizard
	modeCreateEffort  // effort selection step of create wizard
	modeCreateMessage // message input step of create wizard
	modeDinoGame      // full-screen dino runner game
	modeDepsStatus    // full-screen deps status (gh / tmux / git / codex)
	modeSearch        // Cmd+K-style fuzzy palette over the agent list
)

// -- Viewport focus --

const (
	focusAgentList = iota
	focusFiles
	focusHistory
	focusMessage
	focusCount // sentinel for wrapping
)

// Layout constants for the right panel viewports.
// Heights are computed proportionally via panelHeights().
const (
	defaultHeaderLines = 9 // estimate for mouse routing & initial sizing; render overrides
	sectionGaps        = 5 // 3 section labels + 2 blank-line buffers between sections
	minFilesHeight     = 3
	minHistoryHeight   = 5
	minMessageHeight   = 5
)

// panelHeights computes proportional heights for the three right-panel
// viewports given the total panel height.  Files 15%, History 30%, Live gets
// the remainder (~55%).
func panelHeights(panelHeight, headerLines int) (filesH, historyH, msgH int) {
	available := panelHeight - headerLines - sectionGaps
	if available < minFilesHeight+minHistoryHeight+minMessageHeight {
		return minFilesHeight, minHistoryHeight, minMessageHeight
	}
	filesH = max(available*15/100, minFilesHeight)
	historyH = max(available*30/100, minHistoryHeight)
	msgH = available - filesH - historyH
	if msgH < minMessageHeight {
		msgH = minMessageHeight
	}
	return filesH, historyH, msgH
}
