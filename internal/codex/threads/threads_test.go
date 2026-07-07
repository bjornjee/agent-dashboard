package threads

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestThreads_ReadsStateDB(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "state_5.sqlite")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
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
		);
		INSERT INTO threads VALUES
			('new', '/repo', 'feat/new', 'new task', '2026-07-06T10:02:00Z', 0),
			('old', '/repo', 'feat/old', 'old task', '2026-07-06T10:01:00Z', 0),
			('archived', '/repo', 'feat/archived', 'archived task', '2026-07-06T10:03:00Z', 1);
	`); err != nil {
		t.Fatalf("seed fixture db: %v", err)
	}

	got, err := Threads(root)
	if err != nil {
		t.Fatalf("Threads: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("threads len = %d, want 3", len(got))
	}
	if got[0].ID != "archived" || !got[0].Archived {
		t.Fatalf("first thread = %+v", got[0])
	}
	if got[1].ID != "new" || got[1].Cwd != "/repo" || got[1].GitBranch != "feat/new" || got[1].FirstUserMessage != "new task" || got[1].Archived {
		t.Fatalf("second thread = %+v", got[1])
	}
}

func TestThreads_MissingFileOrTableReturnsEmpty(t *testing.T) {
	tests := []struct {
		name string
		init func(root string)
	}{
		{name: "missing file"},
		{
			name: "missing table",
			init: func(root string) {
				conn, err := sql.Open("sqlite", filepath.Join(root, "state_5.sqlite"))
				if err != nil {
					t.Fatalf("open fixture db: %v", err)
				}
				defer conn.Close()
				if _, err := conn.Exec("CREATE TABLE other (id TEXT)"); err != nil {
					t.Fatalf("create other table: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.init != nil {
				tt.init(root)
			}
			got, err := Threads(root)
			if err != nil {
				t.Fatalf("Threads: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("threads len = %d, want 0", len(got))
			}
		})
	}
}
