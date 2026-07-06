CREATE TABLE agents (
  session_id       TEXT PRIMARY KEY,
  harness          TEXT NOT NULL DEFAULT 'claude',
  tmux_pane_id     TEXT NOT NULL DEFAULT '',
  tmux_server_pid  TEXT NOT NULL DEFAULT '',
  target           TEXT NOT NULL DEFAULT '',
  cwd              TEXT NOT NULL DEFAULT '',
  branch           TEXT NOT NULL DEFAULT '',
  state            TEXT NOT NULL DEFAULT '',
  report_seq       INTEGER NOT NULL DEFAULT 0,
  updated_at       TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL DEFAULT '', -- first dashboard sync, not agent start time
  payload          TEXT NOT NULL,
  source           TEXT NOT NULL DEFAULT 'hook',
  dismissed_at     TEXT,
  dismissed_reason TEXT
);

CREATE INDEX idx_agents_pane ON agents(tmux_pane_id);
