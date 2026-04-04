package zsuggest

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry represents a single entry from a z-style frecency file.
type Entry struct {
	Path      string
	Rank      float64
	Timestamp int64
}

// sessionFile mirrors the Claude Code session JSON schema.
// Kept local to avoid a dependency on the conversation package.
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
}

// ParseZLine parses a single line from the ~/.z file.
// Format: path|rank|timestamp
func ParseZLine(line string) (Entry, bool) {
	parts := strings.SplitN(line, "|", 3)
	if len(parts) != 3 {
		return Entry{}, false
	}
	path := parts[0]
	if path == "" {
		return Entry{}, false
	}
	rank, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return Entry{}, false
	}
	ts, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Entry{}, false
	}
	return Entry{Path: path, Rank: rank, Timestamp: ts}, true
}

// LoadZEntriesFromFile reads and parses a z-format file.
func LoadZEntriesFromFile(path string) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if entry, ok := ParseZLine(scanner.Text()); ok {
			entries = append(entries, entry)
		}
	}
	_ = scanner.Err() // partial results are acceptable for suggestions
	return entries
}

// LoadSessionEntries reads Claude Code session files and returns unique
// project directories as Entry values, suitable for the suggestion pipeline.
func LoadSessionEntries(sessionsDir string) []Entry {
	dirEntries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	// Deduplicate by cwd, keeping the most recent startedAt.
	best := make(map[string]int64)
	for _, e := range dirEntries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		var sf sessionFile
		if json.Unmarshal(data, &sf) != nil || sf.Cwd == "" {
			continue
		}
		if sf.StartedAt > best[sf.Cwd] {
			best[sf.Cwd] = sf.StartedAt
		}
	}

	entries := make([]Entry, 0, len(best))
	for cwd, startedAt := range best {
		entries = append(entries, Entry{
			Path:      cwd,
			Rank:      1,
			Timestamp: startedAt / 1000, // ms -> seconds
		})
	}
	return entries
}

// LoadZEntriesWithHome loads directory suggestions, trying ~/.z first and
// falling back to Claude Code session history.
func LoadZEntriesWithHome(homeDir, sessionsDir string) []Entry {
	entries := LoadZEntriesFromFile(filepath.Join(homeDir, ".z"))
	if len(entries) > 0 {
		return entries
	}
	return LoadSessionEntries(sessionsDir)
}

// LoadZEntries reads from the default ~/.z file, falling back to session entries.
func LoadZEntries(sessionsDir string) []Entry {
	home, err := os.UserHomeDir()
	if err != nil {
		return LoadSessionEntries(sessionsDir)
	}
	return LoadZEntriesWithHome(home, sessionsDir)
}

// Frecency computes a frecency score combining rank and recency.
// Based on z.sh's algorithm: rank * recency_weight
func Frecency(entry Entry) float64 {
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

// DirExists returns true if the path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// FilterZSuggestions returns up to 5 paths matching the query, ranked by frecency.
// Matching is case-insensitive substring of the path.
// If pathExists is non-nil, entries whose paths fail the check are excluded.
func FilterZSuggestions(query string, entries []Entry, pathExists func(string) bool) []string {
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
			matches = append(matches, scored{path: e.Path, score: Frecency(e)})
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
