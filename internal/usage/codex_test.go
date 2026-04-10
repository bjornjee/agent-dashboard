package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadCodexSession_ParsesTokenCount(t *testing.T) {
	dir := t.TempDir()
	jsonl := `{"timestamp":"2026-04-03T04:29:25.627Z","type":"session_meta","payload":{"id":"abc123"}}
{"timestamp":"2026-04-03T04:29:25.628Z","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"2026-04-03T04:29:25.835Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":10,"total_tokens":1500},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":10,"total_tokens":1500}}}}
{"timestamp":"2026-04-03T04:30:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":2000,"cached_input_tokens":400,"output_tokens":1000,"reasoning_output_tokens":20,"total_tokens":3000},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":500,"reasoning_output_tokens":10,"total_tokens":1500}}}}
`
	path := filepath.Join(dir, "session.jsonl")
	writeFile(t, path, jsonl)

	sess := readCodexSession(path)

	if sess.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want %q", sess.Model, "gpt-5.4")
	}
	// Should use the last cumulative total_token_usage
	if sess.InputTokens != 2000 {
		t.Errorf("InputTokens = %d, want %d", sess.InputTokens, 2000)
	}
	if sess.CachedInputTokens != 400 {
		t.Errorf("CachedInputTokens = %d, want %d", sess.CachedInputTokens, 400)
	}
	if sess.OutputTokens != 1000 {
		t.Errorf("OutputTokens = %d, want %d", sess.OutputTokens, 1000)
	}
	if sess.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0", sess.CostUSD)
	}
}

func TestReadCodexSession_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	writeFile(t, path, "")

	sess := readCodexSession(path)
	if sess.InputTokens != 0 || sess.OutputTokens != 0 || sess.CostUSD != 0 {
		t.Errorf("empty file should produce zero session, got %+v", sess)
	}
}

func TestReadCodexSession_NoTokenEntries(t *testing.T) {
	dir := t.TempDir()
	jsonl := `{"timestamp":"2026-04-03T04:29:25.627Z","type":"session_meta","payload":{"id":"abc123"}}
{"timestamp":"2026-04-03T04:29:25.628Z","type":"turn_context","payload":{"model":"gpt-5.4"}}
`
	path := filepath.Join(dir, "session.jsonl")
	writeFile(t, path, jsonl)

	sess := readCodexSession(path)
	if sess.OutputTokens != 0 {
		t.Errorf("no token entries should produce zero tokens, got %+v", sess)
	}
	if sess.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want %q", sess.Model, "gpt-5.4")
	}
}

func TestReadCodexSession_NullInfo(t *testing.T) {
	dir := t.TempDir()
	// token_count with null info should be skipped
	jsonl := `{"timestamp":"2026-04-03T04:29:25.835Z","type":"event_msg","payload":{"type":"token_count","info":null}}
`
	path := filepath.Join(dir, "session.jsonl")
	writeFile(t, path, jsonl)

	sess := readCodexSession(path)
	if sess.InputTokens != 0 {
		t.Errorf("null info should produce zero tokens, got %+v", sess)
	}
}

func TestLookupCodexPricing(t *testing.T) {
	p := lookupCodexPricing("gpt-5.4")
	if p.Input != 2.50 {
		t.Errorf("gpt-5.4 input = %f, want 2.50", p.Input)
	}
	if p.Output != 15.0 {
		t.Errorf("gpt-5.4 output = %f, want 15.0", p.Output)
	}

	// Unknown model should return gpt-5.4 default
	p2 := lookupCodexPricing("gpt-unknown-999")
	if p2.Input != 2.50 {
		t.Errorf("unknown model input = %f, want 2.50 (default)", p2.Input)
	}
}

func TestReadCodexDailyUsage_ScansDateDirs(t *testing.T) {
	dir := t.TempDir()
	// Create date-partitioned structure: 2026/04/03/
	dateDir := filepath.Join(dir, "2026", "04", "03")
	jsonl := `{"timestamp":"2026-04-03T04:29:25.628Z","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"2026-04-03T04:30:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":2000,"cached_input_tokens":400,"output_tokens":1000,"reasoning_output_tokens":20,"total_tokens":3000}}}}
`
	writeFile(t, filepath.Join(dateDir, "rollout-abc.jsonl"), jsonl)

	// Also add a second session on the same day
	jsonl2 := `{"timestamp":"2026-04-03T05:00:00.000Z","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"2026-04-03T05:01:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":500,"cached_input_tokens":100,"output_tokens":250,"reasoning_output_tokens":5,"total_tokens":750}}}}
`
	writeFile(t, filepath.Join(dateDir, "rollout-def.jsonl"), jsonl2)

	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	days := ReadCodexDailyUsage(dir, since)

	if len(days) != 1 {
		t.Fatalf("expected 1 day, got %d", len(days))
	}
	d := days[0]
	if d.Date != "2026-04-03" {
		t.Errorf("Date = %q, want %q", d.Date, "2026-04-03")
	}
	// Should sum both sessions
	if d.InputTokens != 2500 {
		t.Errorf("InputTokens = %d, want %d", d.InputTokens, 2500)
	}
	if d.OutputTokens != 1250 {
		t.Errorf("OutputTokens = %d, want %d", d.OutputTokens, 1250)
	}
	if d.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0", d.CostUSD)
	}
}

func TestReadCodexDailyUsage_SkipsOldDates(t *testing.T) {
	dir := t.TempDir()
	// Create old date dir that should be skipped
	oldDir := filepath.Join(dir, "2026", "01", "01")
	jsonl := `{"timestamp":"2026-01-01T04:00:00.000Z","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"2026-01-01T04:01:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":500,"reasoning_output_tokens":0,"total_tokens":1500}}}}
`
	writeFile(t, filepath.Join(oldDir, "rollout-old.jsonl"), jsonl)

	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	days := ReadCodexDailyUsage(dir, since)

	if len(days) != 0 {
		t.Errorf("expected 0 days (old data skipped), got %d", len(days))
	}
}

func TestReadCodexDailyUsage_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	days := ReadCodexDailyUsage(dir, since)

	if len(days) != 0 {
		t.Errorf("expected 0 days for empty dir, got %d", len(days))
	}
}

func TestReadCodexDailyUsage_NonexistentDir(t *testing.T) {
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	days := ReadCodexDailyUsage("/nonexistent/path", since)

	if len(days) != 0 {
		t.Errorf("expected 0 days for nonexistent dir, got %d", len(days))
	}
}

func TestCodexCostCalculation(t *testing.T) {
	// gpt-5.4: $2.50/M input, $15.00/M output, $0.25/M cache read
	// 2000 input (400 cached) = 1600 non-cached + 400 cached
	// Cost: 1600/1M * 2.50 + 400/1M * 0.25 + 1000/1M * 15.00
	//     = 0.004 + 0.0001 + 0.015 = 0.0191
	p := lookupCodexPricing("gpt-5.4")
	cost := codexCost(p, 2000, 400, 1000)

	expected := 0.0191
	if cost < expected-0.0001 || cost > expected+0.0001 {
		t.Errorf("cost = %f, want ~%f", cost, expected)
	}
}
