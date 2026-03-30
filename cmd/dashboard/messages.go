package main

import "time"

// -- Messages --

type stateUpdatedMsg struct{ state StateFile }
type tickMsg time.Time
type jumpResultMsg struct{ err error }
type sendResultMsg struct{ err error }
type captureResultMsg struct{ lines []string }
type conversationMsg struct{ entries []ConversationEntry }
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
type notifyResultMsg struct{}
type selectPaneMsg struct{ err error }
type closeResultMsg struct {
	err     error
	renames map[string]string // oldTarget → newTarget (from window renumbering)
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

// Fixed heights for inner viewports
const (
	filesVPHeight   = 5
	historyVPHeight = 10
	headerLines     = 8 // header + state + branch + dir + cost + spacers
	sectionGaps     = 6 // gaps between sections (labels + blank-line buffers)
	bannerHeight    = 6 // top banner: 11 pixel rows rendered via half-blocks
)
