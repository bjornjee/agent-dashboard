package usage

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

type msgFixture struct {
	id  string
	in  int
	out int
}

// writeJSONL writes a Claude-style JSONL fixture with N distinct message.id
// entries, each repeated `repeat` times (simulating the content-block
// duplication that triggers the double-counting bug).
func writeJSONL(t *testing.T, dir, sessionID string, msgs []msgFixture, repeat int) {
	t.Helper()
	path := filepath.Join(dir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer f.Close()
	for _, m := range msgs {
		line := `{"type":"assistant","message":{"id":"` + m.id +
			`","model":"claude-sonnet-4-6","role":"assistant","content":[],"usage":{"input_tokens":` +
			strconv.Itoa(m.in) + `,"output_tokens":` + strconv.Itoa(m.out) + `}}}` + "\n"
		for i := 0; i < repeat; i++ {
			if _, err := f.WriteString(line); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
		}
	}
}

func TestRecomputeClaudeUsage_ScalesInflatedRows(t *testing.T) {
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	projects := t.TempDir()
	const sessID = "sess-A"
	// Two msgids, each duplicated 2x → inflated parser would have reported 2x.
	writeJSONL(t, projects, sessID, []msgFixture{
		{id: "msg_1", in: 100, out: 50},
		{id: "msg_2", in: 200, out: 100},
	}, 2)

	// Seed two daily_usage rows that look like the inflated cumulative+delta
	// split would have produced: cumulative cost (sonnet) ≈ 0.006300, split
	// 2:1 across day1 and day2.
	inflatedRow1 := domain.Usage{InputTokens: 400, OutputTokens: 200, CostUSD: 0.004200}
	inflatedRow2 := domain.Usage{InputTokens: 200, OutputTokens: 100, CostUSD: 0.002100}
	if err := database.UpsertUsageWithProvider("2026-05-20", sessID, "claude", "claude-sonnet-4-6", inflatedRow1); err != nil {
		t.Fatalf("seed row1: %v", err)
	}
	if err := database.UpsertUsageWithProvider("2026-05-21", sessID, "claude", "claude-sonnet-4-6", inflatedRow2); err != nil {
		t.Fatalf("seed row2: %v", err)
	}

	resolver := func(_, sid string) string {
		if sid == sessID {
			return projects
		}
		return ""
	}

	if err := RecomputeClaudeUsage(database, projects, resolver); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	rows := database.SessionDaysForProvider(sessID, "claude")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Cumulative cost after recompute ≈ 0.003150 (half of seeded 0.006300).
	wantTotal := 0.003150
	wantDay1 := wantTotal * (0.004200 / 0.006300)
	wantDay2 := wantTotal * (0.002100 / 0.006300)

	gotDay1, gotDay2 := rows[0].CostUSD, rows[1].CostUSD
	if math.Abs(gotDay1-wantDay1) > 0.000001 {
		t.Errorf("day1 cost: got %f, want %f", gotDay1, wantDay1)
	}
	if math.Abs(gotDay2-wantDay2) > 0.000001 {
		t.Errorf("day2 cost: got %f, want %f", gotDay2, wantDay2)
	}
	if rows[0].InputTokens != 200 {
		t.Errorf("day1 input_tokens: got %d, want 200 (400/2)", rows[0].InputTokens)
	}
}

func TestRecomputeClaudeUsage_SkipsUnresolvable(t *testing.T) {
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	const sessID = "lost-session"
	seed := domain.Usage{InputTokens: 999, CostUSD: 9.99}
	if err := database.UpsertUsageWithProvider("2026-05-20", sessID, "claude", "model", seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resolver := func(_, _ string) string { return "" }

	if err := RecomputeClaudeUsage(database, "/nonexistent", resolver); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	rows := database.SessionDaysForProvider(sessID, "claude")
	if len(rows) != 1 || rows[0].CostUSD != 9.99 {
		t.Errorf("unresolvable session was modified: rows=%+v", rows)
	}
}

func TestRecomputeClaudeUsage_IgnoresCodexRows(t *testing.T) {
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	seed := domain.Usage{InputTokens: 100, CostUSD: 1.23}
	if err := database.UpsertUsageWithProvider("2026-05-20", "codex-daily", "codex", "", seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	called := false
	resolver := func(_, _ string) string {
		called = true
		return ""
	}

	if err := RecomputeClaudeUsage(database, "/tmp", resolver); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	if called {
		t.Errorf("resolver was called for codex row")
	}
	rows := database.SessionDaysForProvider("codex-daily", "codex")
	if len(rows) != 1 || rows[0].CostUSD != 1.23 {
		t.Errorf("codex row modified: %+v", rows)
	}
}

func TestRecomputeClaudeUsageOnce_MarkerPreventsRerun(t *testing.T) {
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	projects := t.TempDir()
	const sessID = "sess-X"
	writeJSONL(t, projects, sessID, []msgFixture{{id: "msg_1", in: 100, out: 50}}, 2)

	// Seed inflated.
	if err := database.UpsertUsageWithProvider("2026-05-20", sessID, "claude", "claude-sonnet-4-6", domain.Usage{InputTokens: 200, OutputTokens: 100, CostUSD: 0.001500}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resolver := func(_, _ string) string { return projects }

	if err := RecomputeClaudeUsageOnce(database, projects, resolver); err != nil {
		t.Fatalf("recompute once: %v", err)
	}
	// Tamper: write an inflated value back; second call must NOT touch it.
	if err := database.UpsertUsageWithProvider("2026-05-20", sessID, "claude", "claude-sonnet-4-6", domain.Usage{InputTokens: 999, OutputTokens: 999, CostUSD: 99.0}); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if err := RecomputeClaudeUsageOnce(database, projects, resolver); err != nil {
		t.Fatalf("recompute once (2nd): %v", err)
	}

	rows := database.SessionDaysForProvider(sessID, "claude")
	if len(rows) != 1 || rows[0].CostUSD != 99.0 {
		t.Errorf("marker did not prevent rerun: %+v", rows)
	}
}
