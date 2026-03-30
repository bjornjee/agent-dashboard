package main

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type zEntry struct {
	Path      string
	Rank      float64
	Timestamp int64
}

// parseZLine parses a single line from the ~/.z file.
// Format: path|rank|timestamp
func parseZLine(line string) (zEntry, bool) {
	parts := strings.SplitN(line, "|", 3)
	if len(parts) != 3 {
		return zEntry{}, false
	}
	path := parts[0]
	if path == "" {
		return zEntry{}, false
	}
	rank, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return zEntry{}, false
	}
	ts, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return zEntry{}, false
	}
	return zEntry{Path: path, Rank: rank, Timestamp: ts}, true
}

// loadZEntriesFromFile reads and parses a z-format file.
func loadZEntriesFromFile(path string) []zEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []zEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if entry, ok := parseZLine(scanner.Text()); ok {
			entries = append(entries, entry)
		}
	}
	_ = scanner.Err() // partial results are acceptable for suggestions
	return entries
}

// loadZEntries reads from the default ~/.z file.
func loadZEntries() []zEntry {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return loadZEntriesFromFile(filepath.Join(home, ".z"))
}

// frecency computes a frecency score combining rank and recency.
// Based on z.sh's algorithm: rank * recency_weight
func frecency(entry zEntry) float64 {
	now := time.Now().Unix()
	dt := now - entry.Timestamp
	switch {
	case dt < 3600: // last hour
		return entry.Rank * 4
	case dt < 86400: // last day
		return entry.Rank * 2
	case dt < 604800: // last week
		return entry.Rank * 0.5
	default:
		return entry.Rank * 0.25
	}
}

// dirExists returns true if the path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// filterZSuggestions returns up to 5 paths matching the query, ranked by frecency.
// Matching is case-insensitive substring of the path.
// If pathExists is non-nil, entries whose paths fail the check are excluded.
func filterZSuggestions(query string, entries []zEntry, pathExists func(string) bool) []string {
	queryLower := strings.ToLower(query)

	type scored struct {
		path  string
		score float64
	}
	var matches []scored

	for _, e := range entries {
		if query == "" || strings.Contains(strings.ToLower(e.Path), queryLower) {
			if pathExists != nil && !pathExists(e.Path) {
				continue
			}
			matches = append(matches, scored{path: e.Path, score: frecency(e)})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	limit := 5
	if len(matches) < limit {
		limit = len(matches)
	}

	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = matches[i].path
	}
	return result
}
