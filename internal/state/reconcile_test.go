package state

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	_ "modernc.org/sqlite"
)

func TestReconcileUnregistered(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"claimed": {SessionID: "claimed", TmuxPaneID: "%1", State: "running"},
	}}
	targets := map[string]domain.PaneTarget{
		"%1": {Session: "s", Window: 1, Pane: 0, Target: "s:1.0"},
		"%2": {Session: "s", Window: 1, Pane: 1, Target: "s:1.1"},
		"%3": {Session: "s", Window: 1, Pane: 2, Target: "s:1.2"},
		"%4": {Session: "s", Window: 1, Pane: 3, Target: "s:1.3"},
	}
	cwds := map[string]string{"%2": "/repo/codex", "%3": "/repo/claude", "%4": "/repo/shell"}
	cmds := map[string]string{"%1": "claude", "%2": "codex-aarch64-a", "%3": "claude", "%4": "zsh"}

	ReconcileUnregistered(&sf, targets, cwds, cmds, now)

	if _, ok := sf.Agents["unregistered-%1"]; ok {
		t.Fatal("claimed pane got placeholder")
	}
	codexAgent, ok := sf.Agents["unregistered-%2"]
	if !ok {
		t.Fatal("codex placeholder missing")
	}
	if codexAgent.Harness != "codex" || codexAgent.Cwd != "/repo/codex" || codexAgent.Target != "s:1.1" {
		t.Fatalf("codex placeholder = %+v", codexAgent)
	}
	claudeAgent, ok := sf.Agents["unregistered-%3"]
	if !ok {
		t.Fatal("claude placeholder missing")
	}
	if claudeAgent.Harness != "claude" || claudeAgent.State != "unregistered" || claudeAgent.UpdatedAt != now.Format(time.RFC3339) {
		t.Fatalf("claude placeholder = %+v", claudeAgent)
	}
	if _, ok := sf.Agents["unregistered-%4"]; ok {
		t.Fatal("non-harness command got placeholder")
	}
}

func TestStoreSync_SkipsUnregisteredPlaceholders(t *testing.T) {
	store, d := testStore(t)
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"unregistered-%2": {
			SessionID:  "unregistered-%2",
			TmuxPaneID: "%2",
			State:      "unregistered",
			UpdatedAt:  "2026-07-06T10:00:00Z",
		},
	}}

	store.Sync(&sf)

	var count int
	if err := d.Conn().Get(&count, "SELECT COUNT(*) FROM agents"); err != nil {
		t.Fatalf("query agents: %v", err)
	}
	if count != 0 {
		t.Fatalf("agent rows = %d, want 0", count)
	}
}

func TestReconcileIdentities_CodexUniqueMatch(t *testing.T) {
	root := seedCodexThreads(t, []codexThreadSeed{
		{id: "known", cwd: "/repo", branch: "feat/known", title: "known task", updatedAt: "2026-07-06T10:03:00Z"},
		{id: "candidate", cwd: "/repo", branch: "feat/candidate", title: "candidate task", updatedAt: "2026-07-06T10:02:00Z"},
	})
	store, d := testStore(t)
	store.Sync(&domain.StateFile{Agents: map[string]domain.Agent{
		"known": {SessionID: "known", State: "running", UpdatedAt: "2026-07-06T10:04:00Z"},
	}})
	if _, err := d.Conn().Exec(
		"INSERT INTO agents (session_id, payload, dismissed_at) VALUES (?, ?, ?)",
		"dismissed", `{"session_id":"dismissed"}`, "2026-07-06T10:00:00Z",
	); err != nil {
		t.Fatalf("seed dismissed row: %v", err)
	}

	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"known": {SessionID: "known", TmuxPaneID: "%1", State: "running"},
		"unregistered-%2": {
			SessionID:  "unregistered-%2",
			TmuxPaneID: "%2",
			Harness:    "codex",
			Cwd:        "/repo",
			State:      "unregistered",
			UpdatedAt:  "2026-07-06T10:05:00Z",
		},
	}}

	ReconcileIdentities(&sf, ReconcileIdentityOptions{Store: store, CodexRoot: root})

	if _, ok := sf.Agents["unregistered-%2"]; ok {
		t.Fatal("placeholder key still present")
	}
	got, ok := sf.Agents["candidate"]
	if !ok {
		t.Fatal("candidate identity not adopted")
	}
	if got.Branch != "feat/candidate" || got.LastMessagePreview != "candidate task" || got.State != "unregistered" {
		t.Fatalf("adopted agent = %+v", got)
	}
}

func TestReconcileIdentities_CodexAmbiguousStaysPlaceholder(t *testing.T) {
	root := seedCodexThreads(t, []codexThreadSeed{
		{id: "one", cwd: "/repo", updatedAt: "2026-07-06T10:02:00Z"},
		{id: "two", cwd: "/repo", updatedAt: "2026-07-06T10:01:00Z"},
	})
	sf := domain.StateFile{Agents: map[string]domain.Agent{
		"unregistered-%2": {
			SessionID:  "unregistered-%2",
			TmuxPaneID: "%2",
			Harness:    "codex",
			Cwd:        "/repo",
			State:      "unregistered",
		},
	}}

	ReconcileIdentities(&sf, ReconcileIdentityOptions{CodexRoot: root})

	if _, ok := sf.Agents["unregistered-%2"]; !ok {
		t.Fatal("ambiguous placeholder was removed")
	}
	if _, ok := sf.Agents["one"]; ok {
		t.Fatal("ambiguous candidate one was adopted")
	}
}

type codexThreadSeed struct {
	id        string
	cwd       string
	branch    string
	title     string
	updatedAt string
	archived  bool
}

func seedCodexThreads(t *testing.T, rows []codexThreadSeed) string {
	t.Helper()
	root := t.TempDir()
	conn, err := sql.Open("sqlite", filepath.Join(root, "state_5.sqlite"))
	if err != nil {
		t.Fatalf("open codex fixture db: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`
		CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			cwd TEXT,
			git_branch TEXT,
			first_user_message TEXT,
			updated_at TEXT,
			archived INTEGER
		)`); err != nil {
		t.Fatalf("create threads table: %v", err)
	}
	for _, row := range rows {
		archived := 0
		if row.archived {
			archived = 1
		}
		if _, err := conn.Exec(
			"INSERT INTO threads (id, cwd, git_branch, first_user_message, updated_at, archived) VALUES (?, ?, ?, ?, ?, ?)",
			row.id, row.cwd, row.branch, row.title, row.updatedAt, archived,
		); err != nil {
			t.Fatalf("insert thread: %v", err)
		}
	}
	return root
}
