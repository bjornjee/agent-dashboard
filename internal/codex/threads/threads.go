package threads

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Thread is a read-only row from Codex's state_5.sqlite threads table.
type Thread struct {
	ID               string `db:"id"`
	Cwd              string `db:"cwd"`
	GitBranch        string `db:"git_branch"`
	FirstUserMessage string `db:"first_user_message"`
	UpdatedAt        string `db:"updated_at"`
	Archived         bool   `db:"archived"`
}

// Threads reads Codex's rebuildable thread index from root/state_5.sqlite.
// Missing files or older schemas without the threads table return an empty
// result so dashboard refreshes stay best-effort.
func Threads(root string) ([]Thread, error) {
	if root == "" {
		return nil, nil
	}
	path := filepath.Join(root, "state_5.sqlite")
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.Query(`
		SELECT id, cwd, git_branch, first_user_message, updated_at, archived
		FROM threads
		ORDER BY updated_at DESC`)
	if err != nil {
		if isMissingThreadsDB(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []Thread
	for rows.Next() {
		var th Thread
		if err := rows.Scan(&th.ID, &th.Cwd, &th.GitBranch, &th.FirstUserMessage, &th.UpdatedAt, &th.Archived); err != nil {
			return nil, err
		}
		out = append(out, th)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isMissingThreadsDB(err error) bool {
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table: threads") ||
		strings.Contains(msg, "unable to open database file")
}
