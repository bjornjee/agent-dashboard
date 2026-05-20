package domain

import "time"

// RateWindow holds utilization data for a single rate-limit window.
type RateWindow struct {
	UsedPercent float64   // 0–100
	ResetsAt    time.Time // zero if unknown
}

// ExtraUsage holds monthly extra-usage (overage) spend data.
type ExtraUsage struct {
	Enabled      bool
	MonthlyLimit float64 // USD
	UsedCredits  float64 // USD
}

// RateLimit holds live rate-limit data from the Anthropic OAuth API.
// Nil fields indicate the window was not present in the API response.
type RateLimit struct {
	Session   *RateWindow
	Weekly    *RateWindow
	Opus      *RateWindow
	Sonnet    *RateWindow
	Extra     *ExtraUsage
	Plan      string // "max", "pro", "team", etc.
	FetchedAt time.Time
}

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
	Effort             string   `json:"effort,omitempty"`
	PermissionMode     string   `json:"permission_mode"`
	SubagentCount      int      `json:"subagent_count"`
	LastHookEvent      string   `json:"last_hook_event"`
	CurrentTool        string   `json:"current_tool"`
	WorktreeCwd        string   `json:"worktree_cwd,omitempty"`
	PRURL              string   `json:"pr_url,omitempty"`
	PinnedState        string   `json:"pinned_state,omitempty"`
	DiagramCount       int      `json:"diagram_count,omitempty"`
	DiagramLatestTS    int64    `json:"diagram_latest_ts,omitempty"`
	// DelegatedPlanToolUseID points at the Agent tool_use_id in the parent
	// session's JSONL whose tool_result is the plan markdown produced by a
	// delegated Plan subagent. Set by agent-state-fast.js on PreToolUse Agent+Plan;
	// cleared when state transitions out of "plan".
	DelegatedPlanToolUseID string `json:"delegated_plan_tool_use_id,omitempty"`

	// ProjDir is the resolved ~/.claude/projects/<slug> directory that
	// contains <SessionID>.jsonl. Populated each load by
	// state.ResolveAgentProjDir; never persisted (slug can drift if the
	// agent's launch cwd disagrees with its first-hook cwd, so consumers
	// must read it fresh). Empty when the JSONL can't be located.
	ProjDir string `json:"-"`

	// Harness names the agent's coding-agent runtime: "claude" (default
	// when omitted, for backward compat with pre-codex state files) or
	// "codex". Conversation/state parsers route on this so the right
	// JSONL schema is used per agent. Written by SessionStart hooks.
	Harness string `json:"harness,omitempty"`
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

// CodexModelPricing holds per-million-token rates in USD for GPT/Codex models.
type CodexModelPricing struct {
	Input     float64
	Output    float64
	CacheRead float64
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
	Harness  Harness  // Active coding-agent harness (claude, codex, ...)
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

// ExperimentalSettings controls opt-in experimental features.
type ExperimentalSettings struct {
	AsciiPet bool `toml:"ascii_pet"` // show animated ASCII pet in left panel
	DinoGame bool `toml:"dino_game"` // enable Chrome-style dino runner game
}

// UsageSettings controls the usage view behavior.
type UsageSettings struct {
	RateLimitPollSeconds int `toml:"rate_limit_poll_seconds"` // how often to fetch rate limits (default 60)
}

// EffortSettings controls the thinking-effort levels the dashboard pins
// at agent spawn time and dispatches on plan-mode transitions. Both the
// Go side (buildAgentCommand for the --effort flag) and the JS hook
// (agent-state-fast for /effort tmux dispatch) read these values so spawn
// baseline and plan-exit baseline stay consistent.
type EffortSettings struct {
	Plan    string `toml:"plan"`    // dispatched on permission_mode='plan' entry
	Default string `toml:"default"` // pinned at spawn and restored on plan exit
}

// CodexHarnessSettings carries codex-CLI-specific spawn knobs. Model,
// Approval, Sandbox, and DefaultReasoningEffort flow into SpawnCommand as
// --model / -a / -s / -c model_reasoning_effort=... flags. Empty fields
// inherit codex's own resolution chain (~/.codex/config.toml > defaults).
type CodexHarnessSettings struct {
	Model                  string `toml:"model"`                    // e.g. "gpt-5.5"
	Approval               string `toml:"approval"`                 // "never" | "untrusted" | "on-request"
	Sandbox                string `toml:"sandbox"`                  // "danger-full-access" | "workspace-write" | ...
	DefaultReasoningEffort string `toml:"default_reasoning_effort"` // "minimal" | "low" | "medium" | "high"
}

// HarnessSettings selects which coding-agent harness backs new agents.
// Default names the harness used when the New Agent flow doesn't override.
// Per-harness subtables hold knobs that only apply to that harness.
type HarnessSettings struct {
	Default string               `toml:"default"` // "claude" | "codex"
	Codex   CodexHarnessSettings `toml:"codex"`
}

// Settings holds all user-facing configuration loaded from settings.toml.
type Settings struct {
	Banner        BannerSettings       `toml:"banner"`
	Notifications NotificationSettings `toml:"notifications"`
	Debug         DebugSettings        `toml:"debug"`
	Experimental  ExperimentalSettings `toml:"experimental"`
	Usage         UsageSettings        `toml:"usage"`
	Effort        EffortSettings       `toml:"effort"`
	Harness       HarnessSettings      `toml:"harness"`
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
