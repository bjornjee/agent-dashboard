package main

import "time"

// -- Messages --

type stateUpdatedMsg struct{ state StateFile }
type tickMsg time.Time
type jumpResultMsg struct{ err error }
type sendResultMsg struct{ err error }
type captureResultMsg struct{ lines []string }
type conversationMsg struct {
	entries    []ConversationEntry
	fileOffset int64  // byte offset after reading JSONL
	sessionKey string // projDir+sessionID for cache invalidation
}
type pruneDeadMsg struct{ removed int }
type usageMsg struct {
	perAgent map[string]Usage
	total    Usage
}
type persistResultMsg struct{ err error }
type dbCostMsg struct {
	total     float64
	todayCost float64
}
type activityMsg struct{ entries []ActivityEntry }
type subagentsMsg struct {
	parentTarget string
	agents       []SubagentInfo
}
type planMsg struct{ content string }
type openEditorMsg struct{ err error }
type selectPaneMsg struct{ err error }
type closeResultMsg struct {
	err error
}
type createSessionMsg struct {
	target string
	err    error
}

// -- Modes --

const (
	modeNormal = iota
	modeReply
	modeUsage
	modeConfirmClose
	modeCreateFolder
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
	headerLines  = 8 // header + state + branch + dir + cost + spacers
	sectionGaps  = 6 // gaps between sections (labels + blank-line buffers)
	bannerHeight = 6 // top banner: 11 pixel rows rendered via half-blocks

	minFilesHeight   = 3
	minHistoryHeight = 5
	minMessageHeight = 5
)

// panelHeights computes proportional heights for the three right-panel
// viewports given the total panel height.  Files 15%, History 30%, Live gets
// the remainder (~55%).
func panelHeights(panelHeight int) (filesH, historyH, msgH int) {
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
