package domain

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
	TmuxPaneID         string   `json:"tmux_pane_id"`
	StartedAt          string   `json:"started_at"`
	UpdatedAt          string   `json:"updated_at"`
	LastMessagePreview string   `json:"last_message_preview"`
	FilesChanged       []string `json:"files_changed"`
	Model              string   `json:"model"`
	PermissionMode     string   `json:"permission_mode"`
	SubagentCount      int      `json:"subagent_count"`
	LastHookEvent      string   `json:"last_hook_event"`
	CurrentTool        string   `json:"current_tool"`
	WorktreeCwd        string   `json:"worktree_cwd,omitempty"`
	PRURL              string   `json:"pr_url,omitempty"`
	PinnedState        string   `json:"pinned_state,omitempty"`
}

// EffectiveDir returns the best directory for git operations and editor opening.
// Prefers WorktreeCwd (agent may be in a worktree) over Cwd (launch directory).
func (a Agent) EffectiveDir() string {
	if a.WorktreeCwd != "" {
		return a.WorktreeCwd
	}
	return a.Cwd
}

// EffectiveState returns the agent's display state. If a pinned state is set
// (e.g. "pr" or "merged"), it overrides the hook-reported state so that
// user-driven promotions survive while the agent continues working.
func (a Agent) EffectiveState() string {
	if a.PinnedState != "" {
		return a.PinnedState
	}
	return a.State
}

// StateFile is the in-memory aggregate of all per-agent JSON files.
type StateFile struct {
	Agents map[string]Agent `json:"agents"`
}

// StatePriority defines state groups: blocked → waiting → running → review → pr → merged.
// PR and merged are user-driven (pinned) states set by the dashboard.
var StatePriority = map[string]int{
	"permission":  1, // blocked — needs y/n approval
	"plan":        1, // blocked — plan ready for review
	"question":    2, // waiting — needs user reply
	"error":       2, // waiting — needs investigation
	"running":     3,
	"idle_prompt": 4, // review — finished turn, at prompt
	"done":        4, // review — finished task
	"pr":          5, // PR created — waiting on GitHub
	"merged":      6, // branch merged — cleanup
}

// ConversationEntry represents a single turn in the conversation.
type ConversationEntry struct {
	Role           string // "human" or "assistant"
	Content        string
	Timestamp      string
	IsNotification bool // true for task-notification messages and their responses
}

// ActivityEntry represents a single line in the activity log.
type ActivityEntry struct {
	Timestamp string
	Kind      string // "human", "assistant", "tool"
	Content   string
}

// SubagentInfo describes a discovered subagent.
type SubagentInfo struct {
	AgentID     string
	AgentType   string
	Description string
	Completed   bool   // true if the subagent has finished
	StartedAt   string // ISO8601 timestamp from first JSONL entry
}

// RateLimitStatus holds the most recent rate limit info from a session JSONL.
type RateLimitStatus struct {
	Limited   bool
	Message   string // e.g. "You've hit your limit · resets 2pm (Asia/Singapore)"
	Timestamp string
}

// ModelPricing holds per-million-token rates in USD.
type ModelPricing struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Usage holds aggregated token counts and estimated cost for a session.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	Model            string // last seen model
}

// AgentProfile defines how the dashboard discovers and interacts with a coding agent.
type AgentProfile struct {
	Name           string // Display name: "Claude Code"
	Command        string // Binary to launch: "claude"
	ConfigDir      string // Base config dir: ~/.claude
	StateDir       string // Dashboard state: ~/.agent-dashboard
	ProjectsDir    string // Conversations: ~/.claude/projects
	PlansDir       string // Plans: ~/.claude/plans
	SessionsDir    string // Session index: ~/.claude/sessions
	PluginCacheDir string // Plugin cache: ~/.claude/plugins/cache
	HomeDir        string // User home directory
}

// Config holds all dashboard configuration.
type Config struct {
	Profile  AgentProfile
	Username string   // Greeting name
	Editor   string   // Editor command
	Settings Settings // User-facing settings from settings.toml
}

// BannerSettings controls what appears in the top banner.
type BannerSettings struct {
	ShowMascot bool `toml:"show_mascot"`
	ShowQuote  bool `toml:"show_quote"`
}

// NotificationSettings controls desktop notifications sent by adapter hooks.
type NotificationSettings struct {
	Enabled      bool `toml:"enabled"`
	Sound        bool `toml:"sound"`
	SilentEvents bool `toml:"silent_events"`
}

// DebugSettings controls debug/diagnostic features.
type DebugSettings struct {
	KeyLog bool `toml:"key_log"` // write key/mouse/focus events to debug-keys.log
}

// Settings holds all user-facing configuration loaded from settings.toml.
type Settings struct {
	Banner        BannerSettings       `toml:"banner"`
	Notifications NotificationSettings `toml:"notifications"`
	Debug         DebugSettings        `toml:"debug"`
}

// TmuxWindowInfo holds a tmux window's index and name.
type TmuxWindowInfo struct {
	Index int
	Name  string
}

// PaneTarget holds the resolved tmux coordinates for a pane.
type PaneTarget struct {
	Session string
	Window  int
	Pane    int
	Target  string // "session:window.pane"
}
