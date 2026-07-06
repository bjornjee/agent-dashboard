package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func testStore(t *testing.T) (*Store, *db.DB) {
	t.Helper()
	d, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return NewStore(d), d
}

func TestStoreSync_UpsertGuard(t *testing.T) {
	store, d := testStore(t)

	oldAgent := domain.Agent{
		SessionID:  "sess-a",
		Harness:    "claude",
		TmuxPaneID: "%1",
		State:      "running",
		ReportSeq:  2,
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": oldAgent}})

	staleAgent := oldAgent
	staleAgent.State = "done"
	staleAgent.ReportSeq = 1
	staleAgent.UpdatedAt = "2026-07-06T09:59:00Z"
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": staleAgent}})

	var state string
	if err := d.Conn().Get(&state, "SELECT state FROM agents WHERE session_id = 'sess-a'"); err != nil {
		t.Fatalf("query stale state: %v", err)
	}
	if state != "running" {
		t.Fatalf("stale upsert changed state to %q", state)
	}

	newerAgent := oldAgent
	newerAgent.State = "done"
	newerAgent.ReportSeq = 3
	newerAgent.UpdatedAt = "2026-07-06T10:01:00Z"
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": newerAgent}})

	if err := d.Conn().Get(&state, "SELECT state FROM agents WHERE session_id = 'sess-a'"); err != nil {
		t.Fatalf("query newer state: %v", err)
	}
	if state != "done" {
		t.Fatalf("newer upsert state = %q, want done", state)
	}
}

func TestStoreSync_DirtyGateSkipsUnchangedAgents(t *testing.T) {
	store, d := testStore(t)
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"a": {SessionID: "a", State: "running", ReportSeq: 1, UpdatedAt: "2026-07-06T10:00:00Z"},
		"b": {SessionID: "b", State: "running", ReportSeq: 1, UpdatedAt: "2026-07-06T10:00:00Z"},
	}}

	store.Sync(&sf)
	afterFirst := totalChanges(t, d)

	store.Sync(&sf)
	afterSecond := totalChanges(t, d)
	if afterSecond != afterFirst {
		t.Fatalf("unchanged sync wrote %d rows, want 0", afterSecond-afterFirst)
	}

	agent := sf.Agents["b"]
	agent.ReportSeq = 2
	sf.Agents["b"] = agent
	store.Sync(&sf)
	afterBump := totalChanges(t, d)
	if afterBump-afterSecond != 1 {
		t.Fatalf("bumped sync wrote %d rows, want 1", afterBump-afterSecond)
	}
}

func TestStoreSync_ResurrectionRules(t *testing.T) {
	tests := []struct {
		name              string
		incomingUpdatedAt string
		wantDismissed     bool
		wantState         string
	}{
		{
			name:              "stale write swallowed",
			incomingUpdatedAt: "2026-07-06T10:00:00Z",
			wantDismissed:     true,
			wantState:         "running",
		},
		{
			name:              "newer write resurrects",
			incomingUpdatedAt: "2026-07-06T10:02:00Z",
			wantDismissed:     false,
			wantState:         "done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, d := testStore(t)
			base := domain.Agent{
				SessionID:  "sess-a",
				TmuxPaneID: "%1",
				State:      "running",
				UpdatedAt:  "2026-07-06T10:00:00Z",
			}
			store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": base}})
			if _, err := d.Conn().Exec(
				"UPDATE agents SET dismissed_at = ?, dismissed_reason = ? WHERE session_id = ?",
				"2026-07-06T10:01:00Z", "user_dismiss", "sess-a",
			); err != nil {
				t.Fatalf("dismiss seed row: %v", err)
			}

			incoming := base
			incoming.State = "done"
			incoming.UpdatedAt = tt.incomingUpdatedAt
			store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": incoming}})

			var row struct {
				State       string  `db:"state"`
				DismissedAt *string `db:"dismissed_at"`
			}
			if err := d.Conn().Get(&row, "SELECT state, dismissed_at FROM agents WHERE session_id = 'sess-a'"); err != nil {
				t.Fatalf("query row: %v", err)
			}
			if (row.DismissedAt != nil) != tt.wantDismissed {
				t.Fatalf("dismissed = %v, want %v", row.DismissedAt != nil, tt.wantDismissed)
			}
			if row.State != tt.wantState {
				t.Fatalf("state = %q, want %q", row.State, tt.wantState)
			}
		})
	}
}

func TestStoreHydrate_SkipsPaneIDReuseAcrossServerRestart(t *testing.T) {
	store, _ := testStore(t)
	// Agent stamped under a previous tmux server. After a server restart,
	// pane IDs restart from small numbers, so a NEW unrelated pane can get
	// the same %N — pane liveness alone must not resurrect this row.
	stale := domain.Agent{
		SessionID:     "stale",
		TmuxPaneID:    "%40",
		TmuxServerPID: "111",
		State:         "running",
		UpdatedAt:     "2026-07-06T10:00:00Z",
	}
	// Same pane ID but no stamped server PID (pre-upgrade hook file):
	// undecidable, so hydrate keeps trusting pane liveness.
	unstamped := domain.Agent{
		SessionID:  "unstamped",
		TmuxPaneID: "%41",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"stale":     stale,
		"unstamped": unstamped,
	}})

	sf := domain.StateFile{Agents: map[string]domain.Agent{}}
	store.Hydrate(&sf, map[string]bool{"%40": true, "%41": true}, "222")

	if _, ok := sf.Agents["stale"]; ok {
		t.Fatal("row stamped with a previous server PID was hydrated onto a reused pane ID")
	}
	if _, ok := sf.Agents["unstamped"]; !ok {
		t.Fatal("row without a stamped server PID was not hydrated")
	}
}

func TestStoreHydrate_FailsClosedWhenServerPIDUnknown(t *testing.T) {
	store, _ := testStore(t)
	stamped := domain.Agent{
		SessionID:     "stamped",
		TmuxPaneID:    "%1",
		TmuxServerPID: "111",
		State:         "running",
		UpdatedAt:     "2026-07-06T10:00:00Z",
	}
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"stamped": stamped}})

	// Enumeration produced live panes but no server identity: the stamped
	// row's server may be dead and %1 reused — fail closed, don't hydrate.
	sf := domain.StateFile{Agents: map[string]domain.Agent{}}
	store.Hydrate(&sf, map[string]bool{"%1": true}, "")

	if _, ok := sf.Agents["stamped"]; ok {
		t.Fatal("stamped row hydrated although the current server PID is unknown")
	}
}

func TestStoreHydrate_FiltersRows(t *testing.T) {
	store, d := testStore(t)
	live := domain.Agent{
		SessionID:  "live",
		TmuxPaneID: "%1",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	dead := domain.Agent{
		SessionID:  "dead",
		TmuxPaneID: "%2",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	dismissed := domain.Agent{
		SessionID:  "dismissed",
		TmuxPaneID: "%3",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"live":      live,
		"dead":      dead,
		"dismissed": dismissed,
	}})
	if _, err := d.Conn().Exec("UPDATE agents SET dismissed_at = ? WHERE session_id = ?", "2026-07-06T10:01:00Z", "dismissed"); err != nil {
		t.Fatalf("dismiss row: %v", err)
	}

	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"existing": {SessionID: "existing", TmuxPaneID: "%1"},
	}}
	store.Hydrate(&sf, map[string]bool{"%1": true, "%3": true}, "")

	if _, ok := sf.Agents["live"]; !ok {
		t.Fatal("live row was not hydrated")
	}
	if _, ok := sf.Agents["dead"]; ok {
		t.Fatal("dead pane row hydrated")
	}
	if _, ok := sf.Agents["dismissed"]; ok {
		t.Fatal("dismissed row hydrated")
	}
	if got := sf.Agents["existing"].SessionID; got != "existing" {
		t.Fatalf("existing agent overwritten: %q", got)
	}
}

func TestStoreDismiss_RemovesFileAndSoftDeletes(t *testing.T) {
	store, d := testStore(t)
	dir := t.TempDir()
	agent := domain.Agent{
		SessionID:  "sess-a",
		TmuxPaneID: "%1",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	seedAgentJSON(t, dir, agent)
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{"sess-a": agent}})

	if err := store.Dismiss(dir, "sess-a", "user_dismiss"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if _, err := os.Stat(filepath.Join(AgentsDir(dir), "sess-a.json")); !os.IsNotExist(err) {
		t.Fatalf("agent file still exists: %v", err)
	}
	var reason string
	if err := d.Conn().Get(&reason, "SELECT dismissed_reason FROM agents WHERE session_id = 'sess-a'"); err != nil {
		t.Fatalf("query dismissed reason: %v", err)
	}
	if reason != "user_dismiss" {
		t.Fatalf("dismissed reason = %q, want user_dismiss", reason)
	}
}

func TestStoreDismiss_ClearsSyncCacheForResurrection(t *testing.T) {
	store, d := testStore(t)
	dir := t.TempDir()
	// updated_at deliberately ahead of the dismissed_at stamp Dismiss writes
	// with time.Now, so the SQL resurrection rule applies if the write gets
	// through the dirty gate.
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	agent := domain.Agent{
		SessionID:  "sess-a",
		TmuxPaneID: "%1",
		State:      "running",
		UpdatedAt:  future,
	}
	seedAgentJSON(t, dir, agent)
	sf := &domain.StateFile{Agents: map[string]domain.Agent{"sess-a": agent}}
	store.Sync(sf)
	if err := store.Dismiss(dir, "sess-a", "user_dismiss"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	// The hook rewrites the identical payload (same report_seq/updated_at).
	// Dismiss must have dropped the cache entry so this write reaches SQL,
	// where the resurrection rule clears the tombstone.
	store.Sync(sf)

	var dismissed *string
	if err := d.Conn().Get(&dismissed, "SELECT dismissed_at FROM agents WHERE session_id = 'sess-a'"); err != nil {
		t.Fatalf("query dismissed_at: %v", err)
	}
	if dismissed != nil {
		t.Fatalf("dismissed_at = %v, want resurrected (NULL)", *dismissed)
	}
}

func TestStoreSweepDeadRows_TombstonesOrphanRows(t *testing.T) {
	store, d := testStore(t)
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"live":   {SessionID: "live", TmuxPaneID: "%1", UpdatedAt: "2026-07-06T10:00:00Z"},
		"filed":  {SessionID: "filed", TmuxPaneID: "%2", UpdatedAt: "2026-07-06T10:00:00Z"},
		"orphan": {SessionID: "orphan", TmuxPaneID: "%3", UpdatedAt: "2026-07-06T10:00:00Z"},
	}})

	// live: pane alive → kept. filed: pane dead but its hook file still
	// exists (restart-survivor) → kept. orphan: pane dead, no file — its
	// deletion happened while no dashboard was running → tombstoned.
	store.SweepDeadRows(map[string]bool{"%1": true}, "", map[string]bool{"filed": true})

	type row struct {
		SessionID       string  `db:"session_id"`
		DismissedAt     *string `db:"dismissed_at"`
		DismissedReason *string `db:"dismissed_reason"`
	}
	var rows []row
	if err := d.Conn().Select(&rows, "SELECT session_id, dismissed_at, dismissed_reason FROM agents ORDER BY session_id"); err != nil {
		t.Fatalf("select: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.SessionID] = r.DismissedAt != nil
		if r.SessionID == "orphan" && (r.DismissedReason == nil || *r.DismissedReason != "dead_pane") {
			t.Fatalf("orphan dismissed_reason = %v, want dead_pane", r.DismissedReason)
		}
	}
	want := map[string]bool{"live": false, "filed": false, "orphan": true}
	for id, dismissed := range want {
		if got[id] != dismissed {
			t.Fatalf("session %q dismissed = %v, want %v", id, got[id], dismissed)
		}
	}
}

func TestStoreSweepDeadRows_NilSafe(t *testing.T) {
	var store *Store
	store.SweepDeadRows(map[string]bool{"%1": true}, "", nil) // must not panic
}

func TestStoreSweepDeadRows_ReusedPaneIDUnderNewServer(t *testing.T) {
	store, d := testStore(t)
	// Row stamped under tmux server 111. After a restart the new server
	// (222) hands out small pane IDs again, so %5 is live — but it is not
	// this row's pane. A reused ID must not protect a dead server's row:
	// left undismissed, it stays a valid identity-adoption candidate.
	stale := domain.Agent{
		SessionID:     "stale",
		TmuxPaneID:    "%5",
		TmuxServerPID: "111",
		State:         "running",
		UpdatedAt:     "2026-07-06T10:00:00Z",
	}
	// Unstamped row on a live pane: undecidable, kept.
	unstamped := domain.Agent{
		SessionID:  "unstamped",
		TmuxPaneID: "%6",
		State:      "running",
		UpdatedAt:  "2026-07-06T10:00:00Z",
	}
	// Stamped under the current server, pane live: kept.
	current := domain.Agent{
		SessionID:     "current",
		TmuxPaneID:    "%7",
		TmuxServerPID: "222",
		State:         "running",
		UpdatedAt:     "2026-07-06T10:00:00Z",
	}
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"stale":     stale,
		"unstamped": unstamped,
		"current":   current,
	}})

	store.SweepDeadRows(map[string]bool{"%5": true, "%6": true, "%7": true}, "222", nil)

	var dismissedIDs []string
	if err := d.Conn().Select(&dismissedIDs, "SELECT session_id FROM agents WHERE dismissed_at IS NOT NULL ORDER BY session_id"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(dismissedIDs) != 1 || dismissedIDs[0] != "stale" {
		t.Fatalf("dismissed = %v, want [stale]", dismissedIDs)
	}
}

func TestStoreGC_RemovesOldDismissedRows(t *testing.T) {
	store, d := testStore(t)
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"old": {SessionID: "old", TmuxPaneID: "%1", UpdatedAt: "2026-07-06T10:00:00Z"},
		"new": {SessionID: "new", TmuxPaneID: "%2", UpdatedAt: "2026-07-06T10:00:00Z"},
	}})
	if _, err := d.Conn().Exec(
		"UPDATE agents SET dismissed_at = CASE session_id WHEN 'old' THEN ? ELSE ? END",
		time.Now().Add(-31*24*time.Hour).UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed dismissed rows: %v", err)
	}

	store.GC()

	var count int
	if err := d.Conn().Get(&count, "SELECT COUNT(*) FROM agents WHERE session_id = 'old'"); err != nil {
		t.Fatalf("query old row: %v", err)
	}
	if count != 0 {
		t.Fatalf("old row count = %d, want 0", count)
	}
	if err := d.Conn().Get(&count, "SELECT COUNT(*) FROM agents WHERE session_id = 'new'"); err != nil {
		t.Fatalf("query new row: %v", err)
	}
	if count != 1 {
		t.Fatalf("new row count = %d, want 1", count)
	}
}

func TestStoreNilSafe(t *testing.T) {
	var nilStore *Store
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"sess-a": {SessionID: "sess-a", UpdatedAt: "2026-07-06T10:00:00Z"},
	}}
	nilStore.Sync(&sf)
	nilStore.Hydrate(&sf, map[string]bool{"%1": true}, "")
	nilStore.GC()

	store := NewStore(nil)
	store.Sync(&sf)
	store.Hydrate(&sf, map[string]bool{"%1": true}, "")
	store.GC()
}

func totalChanges(t *testing.T, d *db.DB) int {
	t.Helper()
	var changes int
	if err := d.Conn().Get(&changes, "SELECT total_changes()"); err != nil {
		t.Fatalf("total_changes: %v", err)
	}
	return changes
}
