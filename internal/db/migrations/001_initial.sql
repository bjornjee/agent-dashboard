CREATE TABLE IF NOT EXISTS daily_usage (
    date               TEXT NOT NULL,
    session_id         TEXT NOT NULL,
    model              TEXT DEFAULT '',
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd           REAL NOT NULL DEFAULT 0,
    updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (date, session_id)
);

CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_usage(date);

CREATE TABLE IF NOT EXISTS quotes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    quote      TEXT NOT NULL,
    author     TEXT NOT NULL DEFAULT '',
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS quotes_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
