-- Add provider column (claude, codex) to support multi-provider usage tracking.
-- Recreates daily_usage with provider in the composite primary key.

ALTER TABLE daily_usage RENAME TO daily_usage_old;

CREATE TABLE daily_usage (
    date               TEXT NOT NULL,
    session_id         TEXT NOT NULL,
    provider           TEXT NOT NULL DEFAULT 'claude',
    model              TEXT DEFAULT '',
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd           REAL NOT NULL DEFAULT 0,
    updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (date, session_id, provider)
);

INSERT INTO daily_usage (date, session_id, provider, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, updated_at)
    SELECT date, session_id, 'claude', model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, updated_at
    FROM daily_usage_old;

DROP TABLE daily_usage_old;

CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_usage(date);
CREATE INDEX IF NOT EXISTS idx_daily_provider ON daily_usage(provider);
