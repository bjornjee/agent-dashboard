package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const schema = `
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
`

// DayCost is a single day's aggregated cost.
type DayCost struct {
	Date    string  `db:"date"`
	CostUSD float64 `db:"cost_usd"`
}

// DB wraps sqlx.DB with repository methods for usage tracking.
type DB struct {
	conn *sqlx.DB
}

// OpenDB opens (or creates) the SQLite database and runs migrations.
func OpenDB(path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
	}

	conn, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// WAL mode for better concurrent read/write performance
	conn.MustExec("PRAGMA journal_mode=WAL")
	conn.MustExec(schema)

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// UpsertUsage inserts or replaces a daily usage row for a session.
func (db *DB) UpsertUsage(date, sessionID, model string, u Usage) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO daily_usage
			(date, session_id, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		date, sessionID, model,
		u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens,
		u.CostUSD, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// TotalCost returns the all-time sum of cost_usd across all sessions and days.
func (db *DB) TotalCost() float64 {
	var total float64
	_ = db.conn.Get(&total, "SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage")
	return total
}

// SessionCostExcludingDate returns the total cost stored for a session across all days except the given date.
func (db *DB) SessionCostExcludingDate(sessionID, excludeDate string) (float64, error) {
	var total float64
	err := db.conn.Get(&total, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage
		WHERE session_id = ? AND date != ?`,
		sessionID, excludeDate,
	)
	return total, err
}

// CostForDate returns the total cost across all sessions for a specific date.
func (db *DB) CostForDate(date string) float64 {
	var total float64
	_ = db.conn.Get(&total, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE date = ?`, date)
	return total
}

// RandomQuote returns a random quote that fits within maxLen characters
// (quote + " — " + author), or empty strings if none fit.
func (db *DB) RandomQuote(maxLen int) (quote, author string) {
	var row struct {
		Quote  string `db:"quote"`
		Author string `db:"author"`
	}
	err := db.conn.Get(&row, `
		SELECT quote, author FROM quotes
		WHERE LENGTH(quote) + LENGTH(author) + 3 <= ?
		ORDER BY RANDOM() LIMIT 1`, maxLen)
	if err != nil {
		return "", ""
	}
	return row.Quote, row.Author
}

// QuoteExists returns true if a quote with the same text already exists.
func (db *DB) QuoteExists(quote string) bool {
	var count int
	_ = db.conn.Get(&count, "SELECT COUNT(*) FROM quotes WHERE quote = ?", quote)
	return count > 0
}

// QuoteCount returns the number of cached quotes.
func (db *DB) QuoteCount() int {
	var count int
	_ = db.conn.Get(&count, "SELECT COUNT(*) FROM quotes")
	return count
}

// InsertQuotes bulk-inserts quotes into the cache.
func (db *DB) InsertQuotes(quotes []QuoteRow) error {
	tx, err := db.conn.Beginx()
	if err != nil {
		return err
	}
	for _, q := range quotes {
		_, err := tx.Exec("INSERT INTO quotes (quote, author) VALUES (?, ?)", q.Quote, q.Author)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LastQuoteFetch returns the date string of the last successful quote fetch, or "".
func (db *DB) LastQuoteFetch() string {
	var val string
	_ = db.conn.Get(&val, "SELECT value FROM quotes_meta WHERE key = 'last_fetch'")
	return val
}

// SetLastQuoteFetch records today as the last fetch date.
func (db *DB) SetLastQuoteFetch(date string) {
	_, _ = db.conn.Exec("INSERT OR REPLACE INTO quotes_meta (key, value) VALUES ('last_fetch', ?)", date)
}

// QuoteRow is a quote to insert into the cache.
type QuoteRow struct {
	Quote  string
	Author string
}

// CostByDay returns daily aggregated cost since the given time, ordered by date.
func (db *DB) CostByDay(since time.Time) []DayCost {
	var days []DayCost
	_ = db.conn.Select(&days, `
		SELECT date, SUM(cost_usd) as cost_usd
		FROM daily_usage
		WHERE date >= ?
		GROUP BY date
		ORDER BY date`,
		since.Format("2006-01-02"),
	)
	return days
}
