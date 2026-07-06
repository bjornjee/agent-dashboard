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
	for _, item := range dirty {
		agent := item.agent
		if _, err := tx.Exec(`
			INSERT INTO agents (
				session_id, harness, tmux_pane_id, target, cwd, branch, state,
				report_seq, updated_at, payload, source, dismissed_at, dismissed_reason
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
			ON CONFLICT(session_id) DO UPDATE SET
				harness = excluded.harness,
				tmux_pane_id = excluded.tmux_pane_id,
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
			defaultHarness(agent.Harness),
			agent.TmuxPaneID,
			agent.Target,
			agent.Cwd,
			agent.Branch,
			agent.State,
			reportSeq(agent),
			agent.UpdatedAt,
			item.payload,
			item.source,
		); err != nil {
			_ = tx.Rollback()
			return
		}
	}
	if err := tx.Commit(); err != nil {
		return
	}
	for _, item := range dirty {
		s.lastSynced[item.agent.SessionID] = item.seq
	}
}

// Hydrate injects non-dismissed read-model rows whose panes are live and whose
// session IDs are missing from the file state.
func (s *Store) Hydrate(sf *domain.StateFile, livePanes map[string]bool) {
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

func defaultHarness(harness string) string {
	if harness == "" {
		return "claude"
	}
	return harness
}

func reportSeq(agent domain.Agent) int {
	return agent.ReportSeq
}

func syncSeq(agent domain.Agent) agentSyncSeq {
	return agentSyncSeq{reportSeq: agent.ReportSeq, updatedAt: agent.UpdatedAt}
}
