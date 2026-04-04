package conversation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func TestProjectSlug(t *testing.T) {
	tests := []struct {
		cwd  string
		want string
	}{
		{"/Users/bjornjee/Code/bjornjee/skills", "-Users-bjornjee-Code-bjornjee-skills"},
		{"/Users/bjornjee/Code/newb/ctf", "-Users-bjornjee-Code-newb-ctf"},
		{"/tmp/test", "-tmp-test"},
		{"/Users/bjornjee/.dotfiles/dotfiles", "-Users-bjornjee--dotfiles-dotfiles"},
		{"/Users/bjornjee/.claude", "-Users-bjornjee--claude"},
		{"/Users/bjornjee/.tmux/plugins/agent-dashboard", "-Users-bjornjee--tmux-plugins-agent-dashboard"},
	}
	for _, tt := range tests {
		got := ProjectSlug(tt.cwd)
		if got != tt.want {
			t.Errorf("ProjectSlug(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}

func TestReadConversation_MissingFile(t *testing.T) {
	entries := ReadConversation("/nonexistent", "no-such-id", 10)
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d entries", len(entries))
	}
}

func TestReadConversation_ParsesEntries(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	jsonl := `{"type":"user","message":{"role":"user","content":"fix the bug"},"timestamp":"2026-03-28T10:15:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"let me think..."},{"type":"text","text":"I fixed the bug by updating the handler."}]},"timestamp":"2026-03-28T10:15:30Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-03-28T10:15:35Z"}
{"type":"user","message":{"role":"user","content":"thanks!"},"timestamp":"2026-03-28T10:16:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"You're welcome!"}]},"timestamp":"2026-03-28T10:16:30Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 10)

	// Should have 4 entries: 2 user + 2 assistant text (skip tool_use-only entry)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %+v", len(entries), entries)
	}

	// First entry: user
	if entries[0].Role != "human" || entries[0].Content != "fix the bug" {
		t.Errorf("entry 0: got %+v", entries[0])
	}

	// Second entry: assistant (thinking stripped, only text)
	if entries[1].Role != "assistant" || entries[1].Content != "I fixed the bug by updating the handler." {
		t.Errorf("entry 1: got %+v", entries[1])
	}

	// Third: user
	if entries[2].Role != "human" || entries[2].Content != "thanks!" {
		t.Errorf("entry 2: got %+v", entries[2])
	}

	// Fourth: assistant
	if entries[3].Role != "assistant" || entries[3].Content != "You're welcome!" {
		t.Errorf("entry 3: got %+v", entries[3])
	}
}

func TestReadConversation_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	jsonl := `{"type":"user","message":{"role":"user","content":"msg1"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"reply1"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"user","message":{"role":"user","content":"msg2"},"timestamp":"2026-03-28T10:01:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"reply2"}]},"timestamp":"2026-03-28T10:01:01Z"}
{"type":"user","message":{"role":"user","content":"msg3"},"timestamp":"2026-03-28T10:02:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"reply3"}]},"timestamp":"2026-03-28T10:02:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Should be the LAST 2 entries
	if entries[0].Content != "msg3" {
		t.Errorf("expected msg3, got %s", entries[0].Content)
	}
	if entries[1].Content != "reply3" {
		t.Errorf("expected reply3, got %s", entries[1].Content)
	}
}

func TestReadConversation_HandlesUserContentArray(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	// User messages with tool_result content (array format) should be skipped
	jsonl := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"tool output"}]}]},"timestamp":"2026-03-28T10:15:00Z"}
{"type":"user","message":{"role":"user","content":"actual user message"},"timestamp":"2026-03-28T10:16:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 10)
	// Only the string-content user message should appear
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Content != "actual user message" {
		t.Errorf("expected 'actual user message', got %q", entries[0].Content)
	}
}

func TestHasPendingToolUse_NoPending(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Assistant sends tool_use, then user sends tool_result -> not pending
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"file.go"}]}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingToolUse(projDir, sessionID) {
		t.Error("expected no pending tool_use, but got true")
	}
}

func TestHasPendingToolUse_Pending(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Assistant sends tool_use with no subsequent tool_result -> pending
	jsonl := `{"type":"user","message":{"role":"user","content":"fix the bug"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix it."},{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"foo.go"}}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if !HasPendingToolUse(projDir, sessionID) {
		t.Error("expected pending tool_use, but got false")
	}
}

func TestHasPendingToolUse_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(""), 0644)

	if HasPendingToolUse(projDir, sessionID) {
		t.Error("expected false for empty file")
	}
}

func TestHasPendingToolUse_MissingFile(t *testing.T) {
	if HasPendingToolUse("/nonexistent", "no-such") {
		t.Error("expected false for missing file")
	}
}

func TestHasPendingToolUse_TextOnlyAssistant(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Last assistant message has only text, no tool_use -> not pending
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done!"}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingToolUse(projDir, sessionID) {
		t.Error("expected no pending tool_use for text-only assistant message")
	}
}

func TestReadConversation_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	jsonl := `not json at all
{"type":"user","message":{"role":"user","content":"valid"},"timestamp":"2026-03-28T10:15:00Z"}
{"broken json
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestFindSubagents_SortedByStartTimeDescending(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-1"
	subDir := filepath.Join(dir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	// Create 3 subagents with different start times
	agents := []struct {
		id        string
		agentType string
		desc      string
		timestamp string // first JSONL entry timestamp
	}{
		{"aaa", "Explore", "oldest agent", "2026-03-28T10:00:00Z"},
		{"bbb", "Bash", "middle agent", "2026-03-28T11:00:00Z"},
		{"ccc", "Plan", "newest agent", "2026-03-28T12:00:00Z"},
	}

	for _, a := range agents {
		meta := subagentMeta{AgentType: a.agentType, Description: a.desc}
		data, _ := json.Marshal(meta)
		os.WriteFile(filepath.Join(subDir, "agent-"+a.id+".meta.json"), data, 0644)

		// JSONL with sessionId and timestamp
		jsonl := `{"type":"system","sessionId":"` + sessionID + `","timestamp":"` + a.timestamp + `"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working"}],"stop_reason":"end_turn"},"timestamp":"` + a.timestamp + `"}
`
		os.WriteFile(filepath.Join(subDir, "agent-"+a.id+".jsonl"), []byte(jsonl), 0644)
	}

	subs := FindSubagents(dir, sessionID)
	if len(subs) != 3 {
		t.Fatalf("expected 3 subagents, got %d", len(subs))
	}

	// Should be sorted newest first: ccc, bbb, aaa
	if subs[0].AgentID != "ccc" {
		t.Errorf("expected first subagent to be 'ccc' (newest), got %q", subs[0].AgentID)
	}
	if subs[1].AgentID != "bbb" {
		t.Errorf("expected second subagent to be 'bbb' (middle), got %q", subs[1].AgentID)
	}
	if subs[2].AgentID != "aaa" {
		t.Errorf("expected third subagent to be 'aaa' (oldest), got %q", subs[2].AgentID)
	}

	// Verify StartedAt is populated
	if subs[0].StartedAt == "" {
		t.Error("expected StartedAt to be populated")
	}
}

func TestMarkNotifications_TagsTaskNotificationPair(t *testing.T) {
	entries := []domain.ConversationEntry{
		{Role: "human", Content: "fix the bug"},
		{Role: "assistant", Content: "I fixed it. Here's the summary of changes."},
		{Role: "human", Content: "<task-notification>\n<task-id>abc123</task-id>\n<status>completed</status>\n</task-notification>"},
		{Role: "assistant", Content: "Background agent completed."},
	}
	markNotifications(entries)

	if entries[0].IsNotification {
		t.Error("regular user message should not be marked as notification")
	}
	if entries[1].IsNotification {
		t.Error("regular assistant message should not be marked as notification")
	}
	if !entries[2].IsNotification {
		t.Error("task-notification user message should be marked as notification")
	}
	if !entries[3].IsNotification {
		t.Error("assistant response after task-notification should be marked as notification")
	}
}

func TestMarkNotifications_MultipleConsecutive(t *testing.T) {
	entries := []domain.ConversationEntry{
		{Role: "human", Content: "do the thing"},
		{Role: "assistant", Content: "Done. All changes committed."},
		{Role: "human", Content: "<task-notification><task-id>a1</task-id></task-notification>"},
		{Role: "assistant", Content: "Agent A completed."},
		{Role: "human", Content: "<task-notification><task-id>b2</task-id></task-notification>"},
		{Role: "assistant", Content: "Agent B completed."},
	}
	markNotifications(entries)

	if entries[0].IsNotification || entries[1].IsNotification {
		t.Error("substantive entries should not be marked")
	}
	for i := 2; i <= 5; i++ {
		if !entries[i].IsNotification {
			t.Errorf("entry %d should be marked as notification", i)
		}
	}
}

func TestMarkNotifications_NotificationAtEnd(t *testing.T) {
	entries := []domain.ConversationEntry{
		{Role: "human", Content: "hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "human", Content: "<task-notification><task-id>x</task-id></task-notification>"},
	}
	markNotifications(entries)

	if !entries[2].IsNotification {
		t.Error("trailing task-notification should still be marked")
	}
	if entries[0].IsNotification || entries[1].IsNotification {
		t.Error("substantive entries should not be marked")
	}
}

func TestReadConversation_MarksNotifications(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	jsonl := `{"type":"user","message":{"role":"user","content":"fix the bug"},"timestamp":"2026-03-28T10:15:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Fixed!"}]},"timestamp":"2026-03-28T10:15:30Z"}
{"type":"user","message":{"role":"user","content":"<task-notification><task-id>bg1</task-id><status>completed</status></task-notification>"},"timestamp":"2026-03-28T10:16:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Background done."}]},"timestamp":"2026-03-28T10:16:30Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 10)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].IsNotification || entries[1].IsNotification {
		t.Error("substantive entries should not be notifications")
	}
	if !entries[2].IsNotification {
		t.Error("task-notification user entry should be marked")
	}
	if !entries[3].IsNotification {
		t.Error("assistant response to task-notification should be marked")
	}
}

func TestIsSubagentCompleted_EndTurn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(path, []byte(jsonl), 0644)
	if !isSubagentCompleted(path) {
		t.Error("expected completed for stop_reason=end_turn")
	}
}

func TestIsSubagentCompleted_ResultType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")
	// Some subagents end with a "result" type entry
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working"}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"result","result":"success","timestamp":"2026-03-28T10:01:00Z"}
`
	os.WriteFile(path, []byte(jsonl), 0644)
	if !isSubagentCompleted(path) {
		t.Error("expected completed for type=result entry")
	}
}

func TestIsSubagentCompleted_MaxTokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ran out"}],"stop_reason":"max_tokens"},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(path, []byte(jsonl), 0644)
	if !isSubagentCompleted(path) {
		t.Error("expected completed for stop_reason=max_tokens")
	}
}

func TestIsSubagentCompleted_StillRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")
	// Last entry is a tool_use with no stop_reason -- still running
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(path, []byte(jsonl), 0644)
	if isSubagentCompleted(path) {
		t.Error("expected not completed for active tool_use")
	}
}

func TestReadConversation_LargeAssistantMessageNotTruncated(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Create a 20000-char assistant message (simulating a plan)
	longText := strings.Repeat("x", 20000)

	msg := map[string]interface{}{
		"role": "assistant",
		"content": []map[string]string{
			{"type": "text", "text": longText},
		},
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entry := `{"type":"assistant","message":` + string(msgJSON) + `,"timestamp":"2026-03-28T10:15:30Z"}`

	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(entry+"\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	entries := ReadConversation(filepath.Join(dir, slug), sessionID, 10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// The full 20000-char message should be preserved, not truncated to 8000
	if len(entries[0].Content) != 20000 {
		t.Errorf("expected content length 20000, got %d (message was truncated)", len(entries[0].Content))
	}
}

func TestReadPlanSlug_ExtractsSlug(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	jsonl := `{"type":"user","message":{"role":"user","content":"fix it"},"timestamp":"2026-03-28T10:00:00Z","slug":"my-cool-plan"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"timestamp":"2026-03-28T10:00:01Z","slug":"my-cool-plan"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	slug := ReadPlanSlug(projDir, sessionID)
	if slug != "my-cool-plan" {
		t.Errorf("expected slug 'my-cool-plan', got %q", slug)
	}
}

func TestReadPlanSlug_NoSlug(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	slug := ReadPlanSlug(projDir, sessionID)
	if slug != "" {
		t.Errorf("expected empty slug, got %q", slug)
	}
}

func TestReadPlanSlug_MissingFile(t *testing.T) {
	slug := ReadPlanSlug("/nonexistent", "no-such")
	if slug != "" {
		t.Errorf("expected empty slug for missing file, got %q", slug)
	}
}

func TestReadPlanSlug_LargeFile(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Create a JSONL file larger than 32KB to trigger tail-seek logic
	var builder strings.Builder
	// Write many entries without slug to pad the file past 32KB
	for i := 0; i < 300; i++ {
		builder.WriteString(fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"padding message %d with extra content to make it longer"},"timestamp":"2026-03-28T10:00:00Z"}`, i))
		builder.WriteByte('\n')
	}
	// Write the slug entries near the end
	builder.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"plan time"}]},"timestamp":"2026-03-28T10:00:01Z","slug":"the-plan-slug"}`)
	builder.WriteByte('\n')

	data := builder.String()
	if len(data) < 32*1024 {
		t.Fatalf("test file too small: %d bytes, need >32KB", len(data))
	}
	t.Logf("JSONL file size: %d bytes", len(data))

	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	slug := ReadPlanSlug(projDir, sessionID)
	if slug != "the-plan-slug" {
		t.Errorf("expected slug 'the-plan-slug' for large file, got %q", slug)
	}
}

func TestReadPlanSlug_LargeFileAllEntriesHaveSlug(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Create a JSONL file larger than 32KB where ALL entries have slug
	var builder strings.Builder
	for i := 0; i < 300; i++ {
		builder.WriteString(fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"message %d with extra content for padding purposes"},"timestamp":"2026-03-28T10:00:00Z","slug":"my-session-slug"}`, i))
		builder.WriteByte('\n')
	}

	data := builder.String()
	if len(data) < 32*1024 {
		t.Fatalf("test file too small: %d bytes, need >32KB", len(data))
	}

	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	slug := ReadPlanSlug(projDir, sessionID)
	if slug != "my-session-slug" {
		t.Errorf("expected 'my-session-slug', got %q", slug)
	}
}

func TestReadPlanSlug_LargeFileSparseSlug(t *testing.T) {
	// Regression test: large file (>32KB) with a single slug entry after
	// the tail seek point. Verifies that the partial-line skip doesn't
	// consume data needed by the scanner.
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	const tailSize = 32 * 1024
	var builder strings.Builder

	// Pad with no-slug entries to push past 32KB total
	padLine := `{"type":"user","message":{"role":"user","content":"` +
		strings.Repeat("x", 200) + `"},"timestamp":"2026-03-28T10:00:00Z"}` + "\n"
	for builder.Len() < tailSize+100 {
		builder.WriteString(padLine)
	}

	// Single slug entry in the tail window
	builder.WriteString(`{"type":"assistant","timestamp":"2026-03-28T10:00:01Z","slug":"sparse-slug"}` + "\n")

	for i := 0; i < 5; i++ {
		builder.WriteString(padLine)
	}

	data := builder.String()
	t.Logf("JSONL file size: %d bytes", len(data))

	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	slug := ReadPlanSlug(projDir, sessionID)
	if slug != "sparse-slug" {
		t.Errorf("expected 'sparse-slug', got %q", slug)
	}
}

func TestReadPlanContent_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	planContent := "# My Plan\n\n## Steps\n1. Do the thing\n2. Test the thing"
	os.MkdirAll(filepath.Join(dir, "plans"), 0755)
	os.WriteFile(filepath.Join(dir, "plans", "my-plan.md"), []byte(planContent), 0644)

	content := ReadPlanContent(filepath.Join(dir, "plans"), "my-plan")
	if content != planContent {
		t.Errorf("expected plan content, got %q", content)
	}
}

func TestReadPlanContent_MissingFile(t *testing.T) {
	content := ReadPlanContent("/nonexistent", "no-plan")
	if content != "" {
		t.Errorf("expected empty for missing file, got %q", content)
	}
}

func TestReadPlanContent_TruncatesLarge(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "plans"), 0755)
	large := strings.Repeat("x", 40000)
	os.WriteFile(filepath.Join(dir, "plans", "big.md"), []byte(large), 0644)

	content := ReadPlanContent(filepath.Join(dir, "plans"), "big")
	if len(content) > 32003 { // truncate adds "\u2026" (3 bytes UTF-8)
		t.Errorf("expected truncated content, got length %d", len(content))
	}
}

func TestIsSubagentCompleted_LargeFinalEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")

	// The real bug: the final assistant message with stop_reason exceeds 4KB.
	// When isSubagentCompleted seeks to (fileSize - 4KB), it lands mid-entry.
	// The partial line fails JSON parsing and no complete line follows.
	largeText := strings.Repeat("x", 6000)

	// Small initial entry + large final entry with stop_reason
	jsonl := `{"type":"system","sessionId":"sess-1","timestamp":"2026-03-28T09:59:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + largeText + `"}],"stop_reason":"end_turn"},"timestamp":"2026-03-28T10:00:00Z"}
`
	if err := os.WriteFile(path, []byte(jsonl), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// File is >4KB but the completion signal is in the final entry which spans beyond 4KB
	if len(jsonl) <= 4*1024 {
		t.Fatalf("test setup error: file too small (%d bytes), need >4KB", len(jsonl))
	}

	if !isSubagentCompleted(path) {
		t.Error("expected completed: large final entry with stop_reason=end_turn should be detected even when entry exceeds 4KB tail buffer")
	}
}

func TestCleanSlashCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "slash command with args",
			input: "<command-message>skills:feature</command-message>\n<command-name>/skills:feature</command-name>\n<command-args>fix the login bug</command-args>",
			want:  "/skills:feature fix the login bug",
		},
		{
			name:  "slash command without args",
			input: "<command-message>skills:feature</command-message>\n<command-name>/skills:feature</command-name>\n<command-args></command-args>",
			want:  "/skills:feature",
		},
		{
			name:  "regular user message unchanged",
			input: "fix the bug",
			want:  "fix the bug",
		},
		{
			name:  "multiline args preserved",
			input: "<command-message>skills:plan</command-message>\n<command-name>/skills:plan</command-name>\n<command-args>refactor auth\nand add tests</command-args>",
			want:  "/skills:plan refactor auth\nand add tests",
		},
		{
			name:  "no command-name tag returns original",
			input: "<command-message>something</command-message>\nbut no command-name",
			want:  "<command-message>something</command-message>\nbut no command-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanSlashCommand(tt.input)
			if got != tt.want {
				t.Errorf("cleanSlashCommand(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConversationEqual(t *testing.T) {
	a := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"},
		{Role: "assistant", Content: "hi", Timestamp: "2026-03-28T10:00:01Z"},
	}
	b := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"},
		{Role: "assistant", Content: "hi", Timestamp: "2026-03-28T10:00:01Z"},
	}
	if !ConversationEqual(a, b) {
		t.Error("identical slices should be equal")
	}
}

func TestConversationEqual_DifferentLength(t *testing.T) {
	a := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"},
	}
	b := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"},
		{Role: "assistant", Content: "hi", Timestamp: "2026-03-28T10:00:01Z"},
	}
	if ConversationEqual(a, b) {
		t.Error("different lengths should not be equal")
	}
}

func TestConversationEqual_DifferentContent(t *testing.T) {
	a := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"},
	}
	b := []domain.ConversationEntry{
		{Role: "human", Content: "goodbye", Timestamp: "2026-03-28T10:00:00Z"},
	}
	if ConversationEqual(a, b) {
		t.Error("different content should not be equal")
	}
}

func TestConversationEqual_DifferentNotification(t *testing.T) {
	a := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z", IsNotification: false},
	}
	b := []domain.ConversationEntry{
		{Role: "human", Content: "hello", Timestamp: "2026-03-28T10:00:00Z", IsNotification: true},
	}
	if ConversationEqual(a, b) {
		t.Error("different IsNotification should not be equal")
	}
}

func TestConversationEqual_BothNil(t *testing.T) {
	if !ConversationEqual(nil, nil) {
		t.Error("both nil should be equal")
	}
}

func TestConversationEqual_BothEmpty(t *testing.T) {
	if !ConversationEqual([]domain.ConversationEntry{}, []domain.ConversationEntry{}) {
		t.Error("both empty should be equal")
	}
}

func TestReadConversationIncremental_FirstRead(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"

	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)

	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	entries, offset := ReadConversationIncremental(filepath.Join(dir, slug), sessionID, 50, nil, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if offset == 0 {
		t.Error("expected non-zero offset after first read")
	}
	if entries[0].Content != "hello" || entries[1].Content != "hi" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestReadConversationIncremental_AppendNew(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"
	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)
	path := filepath.Join(projDir, sessionID+".jsonl")

	// Initial write
	jsonl1 := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(path, []byte(jsonl1), 0644)

	entries, offset := ReadConversationIncremental(projDir, sessionID, 50, nil, 0)
	if len(entries) != 2 {
		t.Fatalf("first read: expected 2 entries, got %d", len(entries))
	}

	// Append more entries
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(`{"type":"user","message":{"role":"user","content":"more"},"timestamp":"2026-03-28T10:01:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]},"timestamp":"2026-03-28T10:01:01Z"}
`)
	f.Close()

	// Incremental read should only parse new entries
	entries2, offset2 := ReadConversationIncremental(projDir, sessionID, 50, entries, offset)
	if len(entries2) != 4 {
		t.Fatalf("second read: expected 4 entries, got %d", len(entries2))
	}
	if offset2 <= offset {
		t.Error("offset should advance")
	}
	if entries2[2].Content != "more" || entries2[3].Content != "ok" {
		t.Errorf("unexpected new entries: %+v", entries2[2:])
	}
}

func TestReadConversationIncremental_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"
	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)
	path := filepath.Join(projDir, sessionID+".jsonl")

	// Write 6 entries, limit 4
	var builder strings.Builder
	for i := 0; i < 6; i++ {
		builder.WriteString(fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"msg%d"},"timestamp":"2026-03-28T10:%02d:00Z"}`, i, i))
		builder.WriteByte('\n')
	}
	os.WriteFile(path, []byte(builder.String()), 0644)

	entries, _ := ReadConversationIncremental(projDir, sessionID, 4, nil, 0)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries (limit), got %d", len(entries))
	}
	// Should be the last 4
	if entries[0].Content != "msg2" {
		t.Errorf("expected msg2 first, got %s", entries[0].Content)
	}
}

func TestReadConversationIncremental_FileShrunk(t *testing.T) {
	dir := t.TempDir()
	slug := "test-project"
	sessionID := "abc-123"
	projDir := filepath.Join(dir, slug)
	os.MkdirAll(projDir, 0755)
	path := filepath.Join(projDir, sessionID+".jsonl")

	// Write data
	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(path, []byte(jsonl), 0644)
	_, offset := ReadConversationIncremental(projDir, sessionID, 50, nil, 0)

	// Truncate and rewrite smaller content
	os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"new"},"timestamp":"2026-03-28T11:00:00Z"}
`), 0644)

	// Should detect shrink and do full re-read
	entries, _ := ReadConversationIncremental(projDir, sessionID, 50, nil, offset)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after shrink, got %d", len(entries))
	}
	if entries[0].Content != "new" {
		t.Errorf("expected 'new', got %s", entries[0].Content)
	}
}

func TestReadConversationIncremental_MissingFile(t *testing.T) {
	entries, offset := ReadConversationIncremental("/nonexistent", "no-such", 50, nil, 0)
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d entries", len(entries))
	}
	if offset != 0 {
		t.Errorf("expected 0 offset, got %d", offset)
	}
}

func TestParseUserEntry_SlashCommand(t *testing.T) {
	content := "<command-message>skills:refactor</command-message>\n<command-name>/skills:refactor</command-name>\n<command-args>clean up the auth module</command-args>"
	contentJSON, _ := json.Marshal(content)
	msgJSON, _ := json.Marshal(map[string]json.RawMessage{
		"role":    json.RawMessage(`"user"`),
		"content": json.RawMessage(contentJSON),
	})
	entry := jsonlEntry{
		Type:      "user",
		Message:   json.RawMessage(msgJSON),
		Timestamp: "2026-03-28T10:00:00Z",
	}

	result := parseUserEntry(entry)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	want := "/skills:refactor clean up the auth module"
	if result.Content != want {
		t.Errorf("got %q, want %q", result.Content, want)
	}
}

// -- HasPendingPlanReview tests --

func TestHasPendingPlanReview_Pending(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// Last assistant turn has ExitPlanMode tool_use, no user response -> pending
	jsonl := `{"type":"user","message":{"role":"user","content":"plan it"},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Here is the plan."},{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if !HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected pending plan review, but got false")
	}
}

func TestHasPendingPlanReview_Approved(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// ExitPlanMode followed by user message -> approved, not pending
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":"yes, go ahead"},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected no pending plan review after user response")
	}
}

func TestHasPendingPlanReview_NoPlan(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// No ExitPlanMode at all
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done!"}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected false for no ExitPlanMode")
	}
}

func TestHasPendingPlanReview_PendingWithToolResult(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// ExitPlanMode followed by tool_result only -> still pending (tool_result is system-generated, not human input)
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted for review"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if !HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected pending plan review -- tool_result is not a human response")
	}
}

func TestHasPendingPlanReview_ApprovedAfterToolResult(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// ExitPlanMode -> tool_result -> human text -> approved
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"user","message":{"role":"user","content":"yes, go ahead"},"timestamp":"2026-03-28T10:00:02Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected no pending plan review -- human approved after tool_result")
	}
}

func TestHasPendingPlanReview_PendingWithAssistantAfterToolResult(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// ExitPlanMode -> tool_result -> assistant text (continuation, no human input) -> still pending
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I have presented my plan for review."}]},"timestamp":"2026-03-28T10:00:02Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if !HasPendingPlanReview(projDir, sessionID) {
		t.Error("expected pending plan review -- assistant text after tool_result is not human approval")
	}
}

func TestHasPendingPlanReview_MissingFile(t *testing.T) {
	if HasPendingPlanReview("/nonexistent", "no-such") {
		t.Error("expected false for missing file")
	}
}

// -- HasPendingQuestion tests --

func TestHasPendingQuestion_Pending(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// AskUserQuestion followed by tool_result only -> still pending
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which approach?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Option A"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if !HasPendingQuestion(projDir, sessionID) {
		t.Error("expected pending question -- tool_result is not a human response")
	}
}

func TestHasPendingQuestion_Answered(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// AskUserQuestion -> tool_result -> human text -> answered
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"Which?"}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"A"}]},"timestamp":"2026-03-28T10:00:01Z"}
{"type":"user","message":{"role":"user","content":"go with A"},"timestamp":"2026-03-28T10:00:02Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingQuestion(projDir, sessionID) {
		t.Error("expected no pending question -- human answered")
	}
}

func TestHasPendingQuestion_NoQuestion(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done!"}]},"timestamp":"2026-03-28T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingQuestion(projDir, sessionID) {
		t.Error("expected false for no AskUserQuestion")
	}
}

func TestHasPendingQuestion_PlanDoesNotTrigger(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0755)
	sessionID := "sess-1"

	// ExitPlanMode should NOT trigger question detection
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"ExitPlanMode","input":{}}]},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"Plan submitted"}]},"timestamp":"2026-03-28T10:00:01Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	if HasPendingQuestion(projDir, sessionID) {
		t.Error("expected false -- ExitPlanMode is not a question")
	}
}
