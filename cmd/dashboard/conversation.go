package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConversationEntry represents a single turn in the conversation.
type ConversationEntry struct {
	Role           string // "human" or "assistant"
	Content        string
	Timestamp      string
	IsNotification bool // true for task-notification messages and their responses
}

// ProjectSlug derives the Claude Code project slug from a cwd path.
// e.g., "/Users/bjornjee/Code/skills" → "-Users-bjornjee-Code-skills"
// Replaces both path separators and dots to match Claude Code's slug convention.
func ProjectSlug(cwd string) string {
	s := strings.ReplaceAll(cwd, string(os.PathSeparator), "-")
	return strings.ReplaceAll(s, ".", "-")
}

// jsonlEntry is the raw structure of a Claude Code session JSONL line.
type jsonlEntry struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp"`
}

type messageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// conversationEqual returns true if two conversation slices have the same
// entries (length, role, content, timestamp, and notification flag).
func conversationEqual(a, b []ConversationEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role ||
			a[i].Content != b[i].Content ||
			a[i].Timestamp != b[i].Timestamp ||
			a[i].IsNotification != b[i].IsNotification {
			return false
		}
	}
	return true
}

// ReadConversation reads the Claude Code session JSONL and returns
// the last `limit` user/assistant text entries.
// projDir is the full path to the project directory under ~/.claude/projects/.
func ReadConversation(projDir, sessionID string, limit int) []ConversationEntry {
	entries, _ := ReadConversationIncremental(projDir, sessionID, limit, nil, 0)
	return entries
}

// ReadConversationIncremental reads conversation entries incrementally.
// On first call, pass prev=nil and prevOffset=0 for a full read.
// On subsequent calls, pass the previous entries and offset to only parse new data.
// Returns the updated entries slice (capped at limit) and the new file offset.
// If the file shrank (truncation/rewrite), it falls back to a full re-read.
func ReadConversationIncremental(projDir, sessionID string, limit int, prev []ConversationEntry, prevOffset int64) ([]ConversationEntry, int64) {
	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	// Check file size for shrink detection
	stat, err := f.Stat()
	if err != nil {
		return nil, 0
	}
	fileSize := stat.Size()

	// If file shrank or offset is invalid, do full re-read
	if prevOffset > fileSize || prevOffset < 0 {
		prevOffset = 0
		prev = nil
	}

	// Nothing new in the file — return previous entries as-is
	if prevOffset > 0 && prevOffset == fileSize && prev != nil {
		return prev, prevOffset
	}

	// Seek to previous offset for incremental read
	if prevOffset > 0 {
		if _, err := f.Seek(prevOffset, 0); err != nil {
			// Seek failed — fall back to full read
			prevOffset = 0
			prev = nil
			if _, err := f.Seek(0, 0); err != nil {
				return nil, 0
			}
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	// For incremental reads, start with previous entries
	var all []ConversationEntry
	prevLen := 0
	if prevOffset > 0 && prev != nil {
		// Snapshot previous entries for incremental append
		all = make([]ConversationEntry, len(prev))
		copy(all, prev)
		prevLen = len(all)
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "user":
			if e := parseUserEntry(entry); e != nil {
				all = append(all, *e)
			}
		case "assistant":
			if e := parseAssistantEntry(entry); e != nil {
				all = append(all, *e)
			}
		}
	}

	// Only mark notifications on new entries + the boundary entry from prev
	// (the boundary entry might be a pending notification awaiting its pair).
	notifStart := prevLen
	if notifStart > 0 {
		notifStart-- // include last prev entry for boundary check
	}
	if notifStart < len(all) {
		markNotifications(all[notifStart:])
	}

	// Cap at limit (keep last N)
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}

	// The scanner reads to EOF; file offset == file size
	return all, fileSize
}

// markNotifications tags task-notification user messages and
// the assistant response that immediately follows each one.
func markNotifications(entries []ConversationEntry) {
	for i := range entries {
		if entries[i].Role == "human" && strings.Contains(entries[i].Content, "<task-notification>") {
			entries[i].IsNotification = true
			for j := i + 1; j < len(entries); j++ {
				if entries[j].Role == "assistant" {
					entries[j].IsNotification = true
					break
				}
			}
		}
	}
}

func parseUserEntry(entry jsonlEntry) *ConversationEntry {
	var env messageEnvelope
	if err := json.Unmarshal(entry.Message, &env); err != nil {
		return nil
	}

	// User content can be a string or an array (tool_result).
	// Only show string content (actual user messages).
	var strContent string
	if err := json.Unmarshal(env.Content, &strContent); err != nil {
		return nil // array content (tool_result) — skip
	}

	strContent = strings.TrimSpace(strContent)
	if strContent == "" {
		return nil
	}
	strContent = cleanSlashCommand(strContent)

	return &ConversationEntry{
		Role:      "human",
		Content:   truncate(strContent, 2000),
		Timestamp: entry.Timestamp,
	}
}

// cleanSlashCommand converts XML-tagged slash command content into a clean
// display format. e.g. "<command-name>/refactor</command-name>\n<command-args>clean up</command-args>"
// becomes "/refactor clean up". Non-slash-command content passes through unchanged.
func cleanSlashCommand(s string) string {
	const nameOpen = "<command-name>"
	const nameClose = "</command-name>"
	const argsOpen = "<command-args>"
	const argsClose = "</command-args>"

	nameStart := strings.Index(s, nameOpen)
	if nameStart < 0 {
		return s
	}
	nameEnd := strings.Index(s, nameClose)
	if nameEnd < 0 {
		return s
	}
	if nameEnd <= nameStart+len(nameOpen) {
		return s
	}
	cmdName := strings.TrimSpace(s[nameStart+len(nameOpen) : nameEnd])

	argsStart := strings.Index(s, argsOpen)
	if argsStart < 0 {
		return cmdName
	}
	argsEnd := strings.Index(s, argsClose)
	if argsEnd < 0 || argsEnd <= argsStart+len(argsOpen) {
		return cmdName
	}
	args := strings.TrimSpace(s[argsStart+len(argsOpen) : argsEnd])
	if args == "" {
		return cmdName
	}
	return cmdName + " " + args
}

func parseAssistantEntry(entry jsonlEntry) *ConversationEntry {
	var env messageEnvelope
	if err := json.Unmarshal(entry.Message, &env); err != nil {
		return nil
	}

	// Assistant content is always an array of blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(env.Content, &blocks); err != nil {
		return nil
	}

	// Extract only "text" blocks, skip "thinking" and "tool_use"
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			texts = append(texts, strings.TrimSpace(b.Text))
		}
	}

	if len(texts) == 0 {
		return nil
	}

	content := strings.Join(texts, "\n")
	return &ConversationEntry{
		Role:      "assistant",
		Content:   truncate(content, 32000),
		Timestamp: entry.Timestamp,
	}
}

// -- Activity Log (includes tool_use entries) --

// ActivityEntry represents a single line in the activity log.
type ActivityEntry struct {
	Timestamp string
	Kind      string // "human", "assistant", "tool"
	Content   string
}

// toolUseBlock is the structure of a tool_use content block.
type toolUseBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	Text  string          `json:"text"`
}

// ReadActivityLog reads a JSONL file and returns activity entries including tool uses.
func ReadActivityLog(jsonlPath string, limit int) []ActivityEntry {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []ActivityEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "user":
			if e := parseUserEntry(entry); e != nil {
				all = append(all, ActivityEntry{
					Timestamp: entry.Timestamp,
					Kind:      "human",
					Content:   e.Content,
				})
			}
		case "assistant":
			entries := parseAssistantActivity(entry)
			all = append(all, entries...)
		}
	}

	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}

// parseAssistantActivity extracts text + tool_use entries from an assistant message.
func parseAssistantActivity(entry jsonlEntry) []ActivityEntry {
	var env messageEnvelope
	if err := json.Unmarshal(entry.Message, &env); err != nil {
		return nil
	}

	var blocks []toolUseBlock
	if err := json.Unmarshal(env.Content, &blocks); err != nil {
		return nil
	}

	var result []ActivityEntry
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text := strings.TrimSpace(b.Text)
			if text != "" {
				result = append(result, ActivityEntry{
					Timestamp: entry.Timestamp,
					Kind:      "assistant",
					Content:   truncate(text, 2000),
				})
			}
		case "tool_use":
			summary := toolSummary(b.Name, b.Input)
			result = append(result, ActivityEntry{
				Timestamp: entry.Timestamp,
				Kind:      "tool",
				Content:   summary,
			})
		}
	}
	return result
}

// toolSummary returns a compact summary like "→ Read: cmd/dashboard/model.go".
func toolSummary(name string, input json.RawMessage) string {
	var m map[string]interface{}
	_ = json.Unmarshal(input, &m)

	detail := ""
	switch name {
	case "Read", "Write", "Edit":
		if fp, ok := m["file_path"].(string); ok {
			detail = shortPath(fp)
		}
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			detail = truncate(cmd, 80)
		}
	case "Grep":
		if pat, ok := m["pattern"].(string); ok {
			detail = truncate(pat, 60)
		}
	case "Glob":
		if pat, ok := m["pattern"].(string); ok {
			detail = pat
		}
	case "Agent":
		if desc, ok := m["description"].(string); ok {
			detail = desc
		}
	default:
		// Generic: show first string value
		for _, v := range m {
			if s, ok := v.(string); ok && s != "" {
				detail = truncate(s, 60)
				break
			}
		}
	}

	if detail != "" {
		return "→ " + name + ": " + detail
	}
	return "→ " + name
}

// shortPath trims home directory prefix for display.
func shortPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// -- Subagent Discovery --

// SubagentInfo describes a discovered subagent.
type SubagentInfo struct {
	AgentID     string
	AgentType   string
	Description string
	Completed   bool   // true if the subagent has finished
	StartedAt   string // ISO8601 timestamp from first JSONL entry
}

// subagentMeta is the JSON structure of agent-<id>.meta.json.
type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// FindSubagents discovers subagents for a session by scanning the subagents directory.
func FindSubagents(projDir, sessionID string) []SubagentInfo {
	subDir := filepath.Join(projDir, sessionID, "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		// Also try flat layout: projDir/subagents/
		subDir = filepath.Join(projDir, "subagents")
		entries, err = os.ReadDir(subDir)
		if err != nil {
			return nil
		}
	}

	var agents []SubagentInfo
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".meta.json") {
			continue
		}

		agentID := strings.TrimPrefix(name, "agent-")
		agentID = strings.TrimSuffix(agentID, ".meta.json")

		// Skip compaction entries (not real subagents)
		if strings.HasPrefix(agentID, "compact-") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(subDir, name))
		if err != nil {
			continue
		}

		var meta subagentMeta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}

		// Verify this subagent belongs to our session by checking JSONL
		jsonlPath := filepath.Join(subDir, "agent-"+agentID+".jsonl")
		if belongsToSession(jsonlPath, sessionID) {
			agents = append(agents, SubagentInfo{
				AgentID:     agentID,
				AgentType:   meta.AgentType,
				Description: meta.Description,
				Completed:   isSubagentCompleted(jsonlPath),
				StartedAt:   subagentStartTime(jsonlPath),
			})
		}
	}

	// Sort by start time descending (newest first)
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].StartedAt > agents[j].StartedAt
	})

	return agents
}

// SubagentJSONLPath returns the path to a subagent's JSONL file.
func SubagentJSONLPath(projDir, sessionID, agentID string) string {
	// Try session-scoped first, then flat
	p := filepath.Join(projDir, sessionID, "subagents", "agent-"+agentID+".jsonl")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(projDir, "subagents", "agent-"+agentID+".jsonl")
}

// isSubagentCompleted checks the tail of a JSONL file for terminal signals:
// - stop_reason of "end_turn" or "max_tokens" in the last assistant message
// - a "result" type entry (subagent returned a result)
func isSubagentCompleted(jsonlPath string) bool {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read last 32KB to find the final lines
	const tailSize = 32 * 1024
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	if stat.Size() > tailSize {
		if _, err := f.Seek(stat.Size()-tailSize, io.SeekStart); err != nil {
			return false
		}
	}

	// Scan all lines in the tail — check each for completion signals
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailSize), 1024*1024) // allow lines up to 1MB

	completed := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			Message struct {
				StopReason string `json:"stop_reason"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}

		// A "result" type entry means the subagent returned
		if entry.Type == "result" {
			completed = true
			continue
		}

		// Check stop_reason on assistant messages
		switch entry.Message.StopReason {
		case "end_turn", "max_tokens":
			completed = true
		}
	}
	return completed
}

// subagentStartTime reads the timestamp from the first JSONL entry.
func subagentStartTime(jsonlPath string) string {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scanner.Scan() {
		var entry struct {
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			return entry.Timestamp
		}
	}
	return ""
}

// belongsToSession checks if a subagent JSONL's sessionId matches the parent.
func belongsToSession(jsonlPath, sessionID string) bool {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scanner.Scan() {
		var entry struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			return entry.SessionID == sessionID
		}
	}
	return false
}

// HasPendingToolUse checks if the last assistant message in the session JSONL
// contains a tool_use block with no subsequent tool_result from the user.
// This indicates the agent is waiting for permission approval.
func HasPendingToolUse(projDir, sessionID string) bool {
	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read the tail of the file (last 32KB should contain recent entries)
	const tailSize = 32 * 1024
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	offset := int64(0)
	if stat.Size() > tailSize {
		offset = stat.Size() - tailSize
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return false
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailSize), tailSize)

	// Track last assistant tool_use and whether a subsequent tool_result appeared
	hasToolUse := false
	toolResultAfter := false

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if json.Unmarshal(line, &entry) != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			// Check if this assistant message contains tool_use blocks
			var env messageEnvelope
			if json.Unmarshal(entry.Message, &env) != nil {
				continue
			}
			var blocks []toolUseBlock
			if json.Unmarshal(env.Content, &blocks) != nil {
				continue
			}
			found := false
			for _, b := range blocks {
				if b.Type == "tool_use" {
					found = true
					break
				}
			}
			if found {
				hasToolUse = true
				toolResultAfter = false // reset — new tool_use seen
			} else {
				hasToolUse = false // text-only assistant message resets
				toolResultAfter = false
			}

		case "user":
			if hasToolUse {
				// Check if this user message contains tool_result
				var env messageEnvelope
				if json.Unmarshal(entry.Message, &env) != nil {
					continue
				}
				// tool_result messages have array content; only need the type field
				var blocks []contentBlock
				if json.Unmarshal(env.Content, &blocks) == nil {
					for _, b := range blocks {
						if b.Type == "tool_result" {
							toolResultAfter = true
							break
						}
					}
				}
			}
		}
	}

	return hasToolUse && !toolResultAfter
}

// RateLimitStatus holds the most recent rate limit info from a session JSONL.
type RateLimitStatus struct {
	Limited   bool
	Message   string // e.g. "You've hit your limit · resets 2pm (Asia/Singapore)"
	Timestamp string
}

// ReadRateLimitStatus scans the tail of a session JSONL for rate_limit errors.
func ReadRateLimitStatus(projDir, sessionID string) RateLimitStatus {
	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return RateLimitStatus{}
	}
	defer f.Close()

	const tailSize = 64 * 1024
	stat, err := f.Stat()
	if err != nil {
		return RateLimitStatus{}
	}
	if stat.Size() > tailSize {
		if _, err := f.Seek(stat.Size()-tailSize, io.SeekStart); err != nil {
			return RateLimitStatus{}
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailSize), tailSize)

	var last RateLimitStatus
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Quick check before full parse
		if !strings.Contains(string(line), "rate_limit") {
			continue
		}

		var entry struct {
			Type      string `json:"type"`
			Error     string `json:"error"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Error != "rate_limit" {
			continue
		}

		// Extract text from content blocks
		var blocks []contentBlock
		if json.Unmarshal(entry.Message.Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "text" && b.Text != "" {
					last = RateLimitStatus{
						Limited:   true,
						Message:   b.Text,
						Timestamp: entry.Timestamp,
					}
					break
				}
			}
		}
	}
	return last
}

// ReadPlanSlug reads the last JSONL entry's slug field for a session.
// Returns empty string if no slug is found or file doesn't exist.
func ReadPlanSlug(projDir, sessionID string) string {
	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read tail of file to find last line with a slug
	const tailSize = 32 * 1024
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	if stat.Size() > tailSize {
		if _, err := f.Seek(stat.Size()-tailSize, io.SeekStart); err != nil {
			return ""
		}
		// Skip the partial first line without buffering ahead.
		// Using bufio.NewReader would read-ahead into its internal buffer,
		// advancing the file offset past data the scanner needs.
		var oneByte [1]byte
		for {
			_, err := f.Read(oneByte[:])
			if err != nil || oneByte[0] == '\n' {
				break
			}
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailSize), tailSize)

	var slug string
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Slug != "" {
			slug = entry.Slug
		}
	}
	return slug
}

// ReadPlanContent reads a plan markdown file from the plans directory.
// Returns empty string if the file doesn't exist.
func ReadPlanContent(plansDir, slug string) string {
	if slug == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(plansDir, slug+".md"))
	if err != nil {
		return ""
	}
	return truncate(string(data), 32000)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// sessionFile represents ~/.claude/sessions/{pid}.json
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
}

// findSessionIDIn finds the most recent session ID for a given cwd
// by scanning sessionsDir/*.json.
func findSessionIDIn(sessionsDir, cwd string) string {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}

	var best sessionFile
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		var sf sessionFile
		if json.Unmarshal(data, &sf) != nil {
			continue
		}
		if sf.Cwd == cwd && sf.StartedAt > best.StartedAt {
			best = sf
		}
	}
	return best.SessionID
}
