package main

import (
	"math"
	"testing"
	"time"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenDB_CreatesTable(t *testing.T) {
	db := testDB(t)

	// Verify table exists by inserting a row
	err := db.UpsertUsage("2026-03-28", "sess-1", "claude-opus-4-6", Usage{
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.01,
	})
	if err != nil {
		t.Fatalf("UpsertUsage: %v", err)
	}
}

func TestUpsertUsage_InsertsAndUpdates(t *testing.T) {
	db := testDB(t)

	// Insert
	err := db.UpsertUsage("2026-03-28", "sess-1", "claude-opus-4-6", Usage{
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.01,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Update same session+date with higher values
	err = db.UpsertUsage("2026-03-28", "sess-1", "claude-opus-4-6", Usage{
		InputTokens:  200,
		OutputTokens: 100,
		CostUSD:      0.02,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Should be one row with updated values
	total := db.TotalCost()
	if math.Abs(total-0.02) > 0.0001 {
		t.Errorf("TotalCost: got %f, want 0.02", total)
	}
}

func TestTotalCost_MultipleSessionsAndDays(t *testing.T) {
	db := testDB(t)

	db.UpsertUsage("2026-03-27", "sess-1", "opus", Usage{CostUSD: 1.50})
	db.UpsertUsage("2026-03-28", "sess-1", "opus", Usage{CostUSD: 2.00})
	db.UpsertUsage("2026-03-28", "sess-2", "sonnet", Usage{CostUSD: 0.50})

	total := db.TotalCost()
	if math.Abs(total-4.00) > 0.0001 {
		t.Errorf("TotalCost: got %f, want 4.00", total)
	}
}

func TestCostByDay(t *testing.T) {
	db := testDB(t)

	db.UpsertUsage("2026-03-26", "sess-1", "opus", Usage{CostUSD: 1.00})
	db.UpsertUsage("2026-03-27", "sess-1", "opus", Usage{CostUSD: 2.00})
	db.UpsertUsage("2026-03-27", "sess-2", "sonnet", Usage{CostUSD: 0.50})
	db.UpsertUsage("2026-03-28", "sess-1", "opus", Usage{CostUSD: 3.00})

	since := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	days := db.CostByDay(since)

	if len(days) != 2 {
		t.Fatalf("CostByDay: got %d days, want 2", len(days))
	}

	// 2026-03-27: 2.00 + 0.50 = 2.50
	if days[0].Date != "2026-03-27" {
		t.Errorf("day 0 date: got %s, want 2026-03-27", days[0].Date)
	}
	if math.Abs(days[0].CostUSD-2.50) > 0.0001 {
		t.Errorf("day 0 cost: got %f, want 2.50", days[0].CostUSD)
	}

	// 2026-03-28: 3.00
	if days[1].Date != "2026-03-28" {
		t.Errorf("day 1 date: got %s, want 2026-03-28", days[1].Date)
	}
	if math.Abs(days[1].CostUSD-3.00) > 0.0001 {
		t.Errorf("day 1 cost: got %f, want 3.00", days[1].CostUSD)
	}
}

func TestTotalCost_EmptyDB(t *testing.T) {
	db := testDB(t)
	total := db.TotalCost()
	if total != 0 {
		t.Errorf("TotalCost on empty DB: got %f, want 0", total)
	}
}

func TestCostByDay_EmptyDB(t *testing.T) {
	db := testDB(t)
	days := db.CostByDay(time.Now().Add(-24 * time.Hour))
	if len(days) != 0 {
		t.Errorf("CostByDay on empty DB: got %d days, want 0", len(days))
	}
}

func TestSessionCostExcludingDate(t *testing.T) {
	db := testDB(t)

	// Session spans two days
	db.UpsertUsage("2026-03-27", "sess-1", "opus", Usage{CostUSD: 5.00})
	db.UpsertUsage("2026-03-28", "sess-1", "opus", Usage{CostUSD: 3.00})

	// Excluding today (03-28), should return only 03-27's cost
	got, err := db.SessionCostExcludingDate("sess-1", "2026-03-28")
	if err != nil {
		t.Fatalf("SessionCostExcludingDate: %v", err)
	}
	if math.Abs(got-5.00) > 0.0001 {
		t.Errorf("SessionCostExcludingDate: got %f, want 5.00", got)
	}

	// Excluding 03-27, should return only 03-28's cost
	got, err = db.SessionCostExcludingDate("sess-1", "2026-03-27")
	if err != nil {
		t.Fatalf("SessionCostExcludingDate: %v", err)
	}
	if math.Abs(got-3.00) > 0.0001 {
		t.Errorf("SessionCostExcludingDate: got %f, want 3.00", got)
	}

	// Non-existent session
	got, err = db.SessionCostExcludingDate("sess-999", "2026-03-28")
	if err != nil {
		t.Fatalf("SessionCostExcludingDate: %v", err)
	}
	if got != 0 {
		t.Errorf("SessionCostExcludingDate for missing session: got %f, want 0", got)
	}
}

func TestCostForDate(t *testing.T) {
	db := testDB(t)
	today := "2026-03-28"

	db.UpsertUsage("2026-03-27", "sess-1", "opus", Usage{CostUSD: 5.00})
	db.UpsertUsage(today, "sess-1", "opus", Usage{CostUSD: 3.00})
	db.UpsertUsage(today, "sess-2", "sonnet", Usage{CostUSD: 1.00})

	got := db.CostForDate(today)
	if math.Abs(got-4.00) > 0.0001 {
		t.Errorf("CostForDate: got %f, want 4.00", got)
	}

	// Empty day
	got = db.CostForDate("2026-03-25")
	if got != 0 {
		t.Errorf("CostForDate for empty day: got %f, want 0", got)
	}
}

func TestDeltaPersistence_NoDuplicateCounting(t *testing.T) {
	db := testDB(t)

	// Simulate a session that runs across two days.
	// Day 1: cumulative cost from JSONL is $5
	// Delta = $5 - $0 (no previous) = $5
	prev, _ := db.SessionCostExcludingDate("sess-1", "2026-03-27")
	delta1 := 5.00 - prev
	db.UpsertUsage("2026-03-27", "sess-1", "opus", Usage{CostUSD: delta1})

	// Day 2: cumulative cost from JSONL is $8
	// Previous days = $5, delta = $8 - $5 = $3
	prev, _ = db.SessionCostExcludingDate("sess-1", "2026-03-28")
	delta2 := 8.00 - prev
	db.UpsertUsage("2026-03-28", "sess-1", "opus", Usage{CostUSD: delta2})

	// Total should be $8 (not $13)
	total := db.TotalCost()
	if math.Abs(total-8.00) > 0.0001 {
		t.Errorf("TotalCost with delta persistence: got %f, want 8.00", total)
	}

	// Day breakdown should be correct
	days := db.CostByDay(time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC))
	if len(days) != 2 {
		t.Fatalf("expected 2 days, got %d", len(days))
	}
	if math.Abs(days[0].CostUSD-5.00) > 0.0001 {
		t.Errorf("day 1 cost: got %f, want 5.00", days[0].CostUSD)
	}
	if math.Abs(days[1].CostUSD-3.00) > 0.0001 {
		t.Errorf("day 2 cost: got %f, want 3.00", days[1].CostUSD)
	}
}
