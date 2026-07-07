package state

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/jmoiron/sqlx"
)

const unregisteredSessionPrefix = "unregistered-"

// Store keeps the SQLite read model in sync with hook-written state files.
type Store struct {
	db         *db.DB
	mu         sync.Mutex
	lastSynced map[string]agentSyncSeq
}

type agentSyncSeq struct {
	reportSeq int
	updatedAt string
}

// NewStore wraps database for state read-model operations.
func NewStore(database *db.DB) *Store {
	return &Store{db: database, lastSynced: make(map[string]agentSyncSeq)}
}

func (s *Store) conn() *sqlx.DB {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Conn()
}

// Sync upserts non-placeholder agents into the SQLite read model.
func (s *Store) Sync(sf *domain.StateFile) {
	conn := s.conn()
	if conn == nil || sf == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	type upsertAgent struct {
		agent   domain.Agent
		payload string
		source  string
		seq     agentSyncSeq
	}
	var dirty []upsertAgent
	for _, agent := range sf.Agents {
		if agent.SessionID == "" || strings.HasPrefix(agent.SessionID, unregisteredSessionPrefix) {
			continue
		}
		seq := syncSeq(agent)
		if s.lastSynced[agent.SessionID] == seq {
			continue
		}
		payloadAgent := agent
		payloadAgent.Resumable = false
		payloadAgent.TrustPromptDetected = false
		payload, err := json.Marshal(payloadAgent)
		if err != nil {
			continue
		}
		source := "hook"
		if agent.State == "unregistered" {
			source = "reconciled"
		}
		dirty = append(dirty, upsertAgent{
			agent:   agent,
			payload: string(payload),
			source:  source,
			seq:     seq,
		})
	}
	if len(dirty) == 0 {
		return
	}

	tx, err := conn.Beginx()
	if err != nil {
		return
	}
	written := make([]upsertAgent, 0, len(dirty))
	for _, item := range dirty {
		agent := item.agent
		res, err := tx.Exec(`
			INSERT INTO agents (
				session_id, harness, tmux_pane_id, tmux_server_pid, target, cwd, branch, state,
				report_seq, updated_at, created_at, payload, source, dismissed_at, dismissed_reason
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
			ON CONFLICT(session_id) DO UPDATE SET
				harness = excluded.harness,
				tmux_pane_id = excluded.tmux_pane_id,
				tmux_server_pid = excluded.tmux_server_pid,
				target = excluded.target,
				cwd = excluded.cwd,
				branch = excluded.branch,
				state = excluded.state,
				report_seq = excluded.report_seq,
				updated_at = excluded.updated_at,
				payload = excluded.payload,
				source = excluded.source,
				dismissed_at = CASE
					WHEN agents.dismissed_at IS NOT NULL AND excluded.updated_at > agents.dismissed_at THEN NULL
					ELSE agents.dismissed_at
				END,
				dismissed_reason = CASE
					WHEN agents.dismissed_at IS NOT NULL AND excluded.updated_at > agents.dismissed_at THEN NULL
					ELSE agents.dismissed_reason
				END
			WHERE
				(agents.dismissed_at IS NULL OR excluded.updated_at > agents.dismissed_at)
				AND (
					excluded.report_seq > agents.report_seq
					OR excluded.updated_at > agents.updated_at
					OR (agents.dismissed_at IS NOT NULL AND excluded.updated_at > agents.dismissed_at)
				)`,
			agent.SessionID,
			domain.HarnessOrDefault(agent.Harness),
			agent.TmuxPaneID,
			agent.TmuxServerPID,
			agent.Target,
			agent.Cwd,
			agent.Branch,
			agent.State,
			reportSeq(agent),
			agent.UpdatedAt,
			time.Now().UTC().Format(time.RFC3339), // created_at: insert-only, conflict update leaves it
			item.payload,
			item.source,
		)
		if err != nil {
			_ = tx.Rollback()
			return
		}
		// Cache means "written", not "attempted": a write the WHERE guard
		// rejected (stale seq, dismissed swallow) must stay dirty so the
		// next cycle re-offers it to SQL.
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			written = append(written, item)
		}
	}
	if err := tx.Commit(); err != nil {
		return
	}
	for _, item := range written {
		s.lastSynced[item.agent.SessionID] = item.seq
	}
}

// Hydrate injects non-dismissed read-model rows whose panes are live and whose
// session IDs are missing from the file state.
func (s *Store) Hydrate(sf *domain.StateFile, livePanes map[string]bool, serverPID string) {
	conn := s.conn()
	if conn == nil || sf == nil || len(livePanes) == 0 {
		return
	}
	if sf.Agents == nil {
		sf.Agents = make(map[string]domain.Agent)
	}
	panes := make([]string, 0, len(livePanes))
	for paneID, live := range livePanes {
		if live {
			panes = append(panes, paneID)
		}
	}
	if len(panes) == 0 {
		return
	}
	query, args, err := sqlx.In(
		"SELECT session_id, payload FROM agents WHERE dismissed_at IS NULL AND tmux_pane_id IN (?)",
		panes,
	)
	if err != nil {
		return
	}
	var rows []struct {
		SessionID string `db:"session_id"`
		Payload   string `db:"payload"`
	}
	if err := conn.Select(&rows, conn.Rebind(query), args...); err != nil {
		return
	}
	for _, row := range rows {
		if row.SessionID == "" {
			continue
		}
		if _, exists := sf.Agents[row.SessionID]; exists {
			continue
		}
		var agent domain.Agent
		if err := json.Unmarshal([]byte(row.Payload), &agent); err != nil {
			continue
		}
		// Compound liveness: pane IDs restart from small numbers after a
		// tmux server restart, so a matching %N alone can be an unrelated
		// pane. A row stamped under a previous server must not resurrect
		// onto a reused ID — and when the current server identity is
		// unknown, stamped rows fail closed. Unstamped rows (pre-upgrade
		// hooks) stay undecidable and keep trusting pane liveness.
		if agent.TmuxServerPID != "" && agent.TmuxServerPID != serverPID {
			continue
		}
		if agent.SessionID == "" {
			agent.SessionID = row.SessionID
		}
		sf.Agents[row.SessionID] = agent
	}
}

// Dismiss soft-deletes an agent row and removes its hook-written JSON file.
func (s *Store) Dismiss(dir, sessionID, reason string) error {
	if conn := s.conn(); conn != nil && sessionID != "" {
		_, _ = conn.Exec(
			"UPDATE agents SET dismissed_at = ?, dismissed_reason = ? WHERE session_id = ?",
			time.Now().UTC().Format(time.RFC3339),
			reason,
			sessionID,
		)
		// Drop the dirty-gate entry so a post-dismissal hook write always
		// reaches SQL, where the resurrection rule decides — a cached seq
		// would otherwise swallow it before the tombstone check runs.
		s.mu.Lock()
		delete(s.lastSynced, sessionID)
		s.mu.Unlock()
	}
	return RemoveAgent(dir, sessionID)
}

// SweepDeadRows tombstones non-dismissed rows whose pane is gone and whose
// hook file no longer exists — deletions that happened while no dashboard
// was running, which the file-based prune can never see. Keeps the read
// model convergent with (files ∪ tmux) across process downtime. livePanes
// must come from a successful enumeration (callers gate on that);
// knownSessions is the set of session IDs currently present as files.
func (s *Store) SweepDeadRows(livePanes map[string]bool, serverPID string, knownSessions map[string]bool) {
	conn := s.conn()
	if conn == nil {
		return
	}
	// Same whole-body locking discipline as Sync: without it a sweep that
	// selected a row before a concurrent Sync upserted it could tombstone
	// the fresh row and drop its cache entry.
	//
	// Asymmetry with Hydrate, by design: with an unknown current server
	// PID, Hydrate fails closed (won't inject a stamped row) while the
	// sweep keeps the row (staleServer can't be established) — unknown
	// identity must never destroy state, only decline to act on it.
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []struct {
		SessionID     string `db:"session_id"`
		TmuxPaneID    string `db:"tmux_pane_id"`
		TmuxServerPID string `db:"tmux_server_pid"`
	}
	if err := conn.Select(&rows, "SELECT session_id, tmux_pane_id, tmux_server_pid FROM agents WHERE dismissed_at IS NULL"); err != nil {
		return
	}
	var orphans []string
	for _, row := range rows {
		if knownSessions[row.SessionID] {
			continue
		}
		// A live pane ID only proves the row's pane exists when the row was
		// stamped under the current server: IDs restart from small numbers
		// after a tmux restart, so a reused %N must not shield a dead
		// server's row — left undismissed it would stay a valid
		// identity-adoption candidate.
		staleServer := row.TmuxServerPID != "" && serverPID != "" && row.TmuxServerPID != serverPID
		if livePanes[row.TmuxPaneID] && !staleServer {
			continue
		}
		orphans = append(orphans, row.SessionID)
	}
	if len(orphans) == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// SQLite caps bound variables (default 999) — batch the IN list.
	for start := 0; start < len(orphans); start += 500 {
		batch := orphans[start:min(start+500, len(orphans))]
		query, args, err := sqlx.In(
			"UPDATE agents SET dismissed_at = ?, dismissed_reason = 'dead_pane' WHERE session_id IN (?)",
			now,
			batch,
		)
		if err != nil {
			return
		}
		_, _ = conn.Exec(conn.Rebind(query), args...)
	}
	for _, id := range orphans {
		delete(s.lastSynced, id)
	}
}

// GC removes old dismissed rows from the read model.
func (s *Store) GC() {
	conn := s.conn()
	if conn == nil {
		return
	}
	// ponytail: 30d fixed TTL, make configurable if anyone asks.
	cutoff := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	_, _ = conn.Exec("DELETE FROM agents WHERE dismissed_at IS NOT NULL AND dismissed_at < ?", cutoff)
}

// Dismissed reports whether sessionID has a soft-delete tombstone.
func (s *Store) Dismissed(sessionID string) bool {
	conn := s.conn()
	if conn == nil || sessionID == "" {
		return false
	}
	var count int
	if err := conn.Get(&count, "SELECT COUNT(*) FROM agents WHERE session_id = ? AND dismissed_at IS NOT NULL", sessionID); err != nil {
		return false
	}
	return count > 0
}

func reportSeq(agent domain.Agent) int {
	return agent.ReportSeq
}

func syncSeq(agent domain.Agent) agentSyncSeq {
	return agentSyncSeq{reportSeq: agent.ReportSeq, updatedAt: agent.UpdatedAt}
}
