package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const maxQuoteLen = 80

type apiNinjasQuote struct {
	Quote  string `json:"quote"`
	Author string `json:"author"`
}

func main() {
	key := os.Getenv("API_NINJAS_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "API_NINJAS_KEY not set")
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = home + "/.agent-dashboard/usage.db"
	}

	conn, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.MustExec("PRAGMA journal_mode=WAL")

	// Count existing quotes
	var existing int
	conn.Get(&existing, "SELECT COUNT(*) FROM quotes")
	fmt.Printf("Existing quotes in DB: %d\n", existing)

	// Load existing quotes for dedup
	seen := make(map[string]bool)
	var existingQuotes []string
	conn.Select(&existingQuotes, "SELECT quote FROM quotes")
	for _, q := range existingQuotes {
		seen[q] = true
	}

	target := 200
	if existing >= target {
		fmt.Printf("Already have %d quotes, target is %d. Done.\n", existing, target)
		return
	}
	needed := target - existing

	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := "https://api.api-ninjas.com/v2/randomquotes"
	params := url.Values{}
	for _, cat := range []string{"wisdom", "philosophy", "life", "humor", "inspirational"} {
		params.Add("categories", cat)
	}
	fullURL := baseURL + "?" + params.Encode()

	added := 0
	skippedLen := 0
	skippedDup := 0
	errors := 0
	maxErrors := 20 // consecutive error limit

	for added < needed {
		req, err := http.NewRequest(http.MethodGet, fullURL, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "request error: %v\n", err)
			break
		}
		req.Header.Set("X-Api-Key", key)

		resp, err := client.Do(req)
		if err != nil {
			errors++
			fmt.Fprintf(os.Stderr, "fetch error (%d): %v\n", errors, err)
			if errors >= maxErrors {
				break
			}
			time.Sleep(time.Second)
			continue
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			fmt.Println("Rate limited, waiting 5s...")
			time.Sleep(5 * time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			errors++
			fmt.Fprintf(os.Stderr, "HTTP %d (%d consecutive errors)\n", resp.StatusCode, errors)
			if errors >= maxErrors {
				break
			}
			time.Sleep(time.Second)
			continue
		}
		errors = 0

		var results []apiNinjasQuote
		json.NewDecoder(resp.Body).Decode(&results)
		resp.Body.Close()

		for _, r := range results {
			fullLen := len(r.Quote) + len(r.Author) + 3 // " — "
			if fullLen > maxQuoteLen {
				skippedLen++
				continue
			}
			if seen[r.Quote] {
				skippedDup++
				continue
			}
			seen[r.Quote] = true
			_, err := conn.Exec("INSERT INTO quotes (quote, author) VALUES (?, ?)", r.Quote, r.Author)
			if err != nil {
				fmt.Fprintf(os.Stderr, "insert error: %v\n", err)
				continue
			}
			added++
			if added%10 == 0 || added == needed {
				fmt.Printf("Progress: %d/%d added (skipped: %d too long, %d dups)\n", added, needed, skippedLen, skippedDup)
			}
		}

		// Small delay to be nice to the API
		time.Sleep(200 * time.Millisecond)
	}

	var total int
	conn.Get(&total, "SELECT COUNT(*) FROM quotes")
	fmt.Printf("\nDone! Total quotes in DB: %d (added %d, skipped %d too long, %d dups)\n", total, added, skippedLen, skippedDup)
}
