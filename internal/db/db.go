package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// DayCost is a single day's aggregated cost.
type DayCost struct {
	Date    string  `db:"date"`
	CostUSD float64 `db:"cost_usd"`
}

// DayUsage is a single day's aggregated token counts and cost.
type DayUsage struct {
	Date             string  `db:"date" json:"date"`
	InputTokens      int     `db:"input_tokens" json:"input_tokens"`
	OutputTokens     int     `db:"output_tokens" json:"output_tokens"`
	CacheReadTokens  int     `db:"cache_read_tokens" json:"cache_read_tokens"`
	CacheWriteTokens int     `db:"cache_write_tokens" json:"cache_write_tokens"`
	CostUSD          float64 `db:"cost_usd" json:"cost_usd"`
}

// DB wraps sqlx.DB with repository methods for usage tracking.
type DB struct {
	conn *sqlx.DB
}

// OpenDB opens (or creates) the SQLite database and runs migrations.
func OpenDB(path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}

	conn, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// WAL mode for better concurrent read/write performance
	conn.MustExec("PRAGMA journal_mode=WAL")

	if err := runMigrations(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// UpsertUsage inserts or replaces a daily usage row for a session (Claude provider).
func (d *DB) UpsertUsage(date, sessionID, model string, u domain.Usage) error {
	return d.UpsertUsageWithProvider(date, sessionID, "claude", model, u)
}

// UpsertUsageWithProvider inserts or replaces a daily usage row for a session with an explicit provider.
func (d *DB) UpsertUsageWithProvider(date, sessionID, provider, model string, u domain.Usage) error {
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO daily_usage
			(date, session_id, provider, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		date, sessionID, provider, model,
		u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens,
		u.CostUSD, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// TotalCost returns the all-time sum of cost_usd across all sessions and days.
func (d *DB) TotalCost() float64 {
	var total float64
	_ = d.conn.Get(&total, "SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage")
	return total
}

// SessionCostExcludingDate returns the total cost stored for a session across all days except the given date.
func (d *DB) SessionCostExcludingDate(sessionID, excludeDate string) (float64, error) {
	return d.SessionCostExcludingDateWithProvider(sessionID, excludeDate, "claude")
}

// SessionCostExcludingDateWithProvider returns the total cost for a session+provider across all days except the given date.
func (d *DB) SessionCostExcludingDateWithProvider(sessionID, excludeDate, provider string) (float64, error) {
	var total float64
	err := d.conn.Get(&total, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage
		WHERE session_id = ? AND date != ? AND provider = ?`,
		sessionID, excludeDate, provider,
	)
	return total, err
}

// CostForDate returns the total cost across all sessions for a specific date.
func (d *DB) CostForDate(date string) float64 {
	var total float64
	_ = d.conn.Get(&total, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE date = ?`, date)
	return total
}

// RandomQuote returns a random quote that fits within maxLen characters
// (quote + " — " + author), or empty strings if none fit.
func (d *DB) RandomQuote(maxLen int) (quote, author string) {
	var row struct {
		Quote  string `db:"quote"`
		Author string `db:"author"`
	}
	err := d.conn.Get(&row, `
		SELECT quote, author FROM quotes
		WHERE LENGTH(quote) + LENGTH(author) + 3 <= ?
		ORDER BY RANDOM() LIMIT 1`, maxLen)
	if err != nil {
		return "", ""
	}
	return row.Quote, row.Author
}

// QuoteExists returns true if a quote with the same text already exists.
func (d *DB) QuoteExists(quote string) bool {
	var count int
	_ = d.conn.Get(&count, "SELECT COUNT(*) FROM quotes WHERE quote = ?", quote)
	return count > 0
}

// QuoteCount returns the number of cached quotes.
func (d *DB) QuoteCount() int {
	var count int
	_ = d.conn.Get(&count, "SELECT COUNT(*) FROM quotes")
	return count
}

// InsertQuotes bulk-inserts quotes into the cache.
func (d *DB) InsertQuotes(quotes []QuoteRow) error {
	tx, err := d.conn.Beginx()
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
func (d *DB) LastQuoteFetch() string {
	var val string
	_ = d.conn.Get(&val, "SELECT value FROM quotes_meta WHERE key = 'last_fetch'")
	return val
}

// SetLastQuoteFetch records today as the last fetch date.
func (d *DB) SetLastQuoteFetch(date string) {
	_, _ = d.conn.Exec("INSERT OR REPLACE INTO quotes_meta (key, value) VALUES ('last_fetch', ?)", date)
}

// QuoteRow is a quote to insert into the cache.
type QuoteRow struct {
	Quote  string
	Author string
}

// UsageByDay returns daily aggregated token counts and cost since the given
// time, ordered by date ascending.
func (d *DB) UsageByDay(since time.Time) []DayUsage {
	var days []DayUsage
	_ = d.conn.Select(&days, `
		SELECT
			date,
			SUM(input_tokens)       AS input_tokens,
			SUM(output_tokens)      AS output_tokens,
			SUM(cache_read_tokens)  AS cache_read_tokens,
			SUM(cache_write_tokens) AS cache_write_tokens,
			SUM(cost_usd)           AS cost_usd
		FROM daily_usage
		WHERE date >= ?
		GROUP BY date
		ORDER BY date`,
		since.Format("2006-01-02"),
	)
	return days
}

// UsageByDayForProvider returns daily aggregated usage for a specific provider.
func (d *DB) UsageByDayForProvider(since time.Time, provider string) []DayUsage {
	var days []DayUsage
	_ = d.conn.Select(&days, `
		SELECT
			date,
			SUM(input_tokens)       AS input_tokens,
			SUM(output_tokens)      AS output_tokens,
			SUM(cache_read_tokens)  AS cache_read_tokens,
			SUM(cache_write_tokens) AS cache_write_tokens,
			SUM(cost_usd)           AS cost_usd
		FROM daily_usage
		WHERE date >= ? AND provider = ?
		GROUP BY date
		ORDER BY date`,
		since.Format("2006-01-02"), provider,
	)
	return days
}

// TotalCostForProvider returns the all-time sum of cost_usd for a specific provider.
func (d *DB) TotalCostForProvider(provider string) float64 {
	var total float64
	_ = d.conn.Get(&total, "SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE provider = ?", provider)
	return total
}

// CostForDateAndProvider returns the total cost for a specific date and provider.
func (d *DB) CostForDateAndProvider(date, provider string) float64 {
	var total float64
	_ = d.conn.Get(&total, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE date = ? AND provider = ?`, date, provider)
	return total
}

// CostByDay returns daily aggregated cost since the given time, ordered by date.
func (d *DB) CostByDay(since time.Time) []DayCost {
	var days []DayCost
	_ = d.conn.Select(&days, `
		SELECT date, SUM(cost_usd) as cost_usd
		FROM daily_usage
		WHERE date >= ?
		GROUP BY date
		ORDER BY date`,
		since.Format("2006-01-02"),
	)
	return days
}
