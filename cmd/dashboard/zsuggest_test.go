package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseZLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOk   bool
		wantPath string
		wantRank float64
		wantTS   int64
	}{
		{"standard entry", "/Users/me/code|100|1774000000", true, "/Users/me/code", 100, 1774000000},
		{"fractional rank", "/tmp/test|0.5|1770000000", true, "/tmp/test", 0.5, 1770000000},
		{"no separators", "invalid line", false, "", 0, 0},
		{"empty string", "", false, "", 0, 0},
		{"empty path", "|100|123", false, "", 0, 0},
		{"path with pipe", "/Users/me/co|de|100|1774000000", false, "", 0, 0}, // SplitN(3) puts "de|100" into rank — fails parse
		{"path with spaces", "/Users/me/my folder|50|1774000000", true, "/Users/me/my folder", 50, 1774000000},
		{"negative rank", "/tmp/neg|-5|1774000000", true, "/tmp/neg", -5, 1774000000},
		{"zero rank", "/tmp/zero|0|1774000000", true, "/tmp/zero", 0, 1774000000},
		{"non-numeric rank", "/tmp/bad|abc|1774000000", false, "", 0, 0},
		{"non-numeric timestamp", "/tmp/bad|100|notanumber", false, "", 0, 0},
		{"only two fields", "/tmp/bad|100", false, "", 0, 0},
		{"trailing newline", "/Users/me/code|100|1774000000\n", false, "", 0, 0}, // \n in timestamp field
		{"large rank", "/tmp/big|999999.99|1774000000", true, "/tmp/big", 999999.99, 1774000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := parseZLine(tt.line)
			if ok != tt.wantOk {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if entry.Path != tt.wantPath {
				t.Errorf("path=%q, want %q", entry.Path, tt.wantPath)
			}
			if entry.Rank != tt.wantRank {
				t.Errorf("rank=%f, want %f", entry.Rank, tt.wantRank)
			}
			if tt.wantTS != 0 && entry.Timestamp != tt.wantTS {
				t.Errorf("timestamp=%d, want %d", entry.Timestamp, tt.wantTS)
			}
		})
	}
}

func TestLoadZEntries(t *testing.T) {
	// Create a temp z file
	dir := t.TempDir()
	zFile := filepath.Join(dir, ".z")
	content := strings.Join([]string{
		"/Users/me/code/skills|200|1774000000",
		"/Users/me/code/other|50|1773000000",
		"/tmp/scratch|10|1770000000",
		"bad line",
	}, "\n")
	if err := os.WriteFile(zFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := loadZEntriesFromFile(zFile)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Path != "/Users/me/code/skills" {
		t.Errorf("first entry path=%q", entries[0].Path)
	}
}

func TestLoadZEntriesFromFile_Empty(t *testing.T) {
	dir := t.TempDir()
	zFile := filepath.Join(dir, ".z")
	if err := os.WriteFile(zFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	entries := loadZEntriesFromFile(zFile)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from empty file, got %d", len(entries))
	}
}

func TestLoadZEntriesFromFile_MissingFile(t *testing.T) {
	entries := loadZEntriesFromFile("/nonexistent/path/.z")
	if entries != nil {
		t.Errorf("expected nil for missing file, got %v", entries)
	}
}

func TestLoadZEntriesFromFile_SkipsBadLines(t *testing.T) {
	dir := t.TempDir()
	zFile := filepath.Join(dir, ".z")
	content := strings.Join([]string{
		"/good/path|100|1774000000",
		"bad line no pipes",
		"",
		"/another/good|50|1773000000",
		"|0|0",
		"/no-timestamp|100",
	}, "\n")
	if err := os.WriteFile(zFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	entries := loadZEntriesFromFile(zFile)
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(entries))
	}
	if entries[0].Path != "/good/path" {
		t.Errorf("first entry path=%q, want /good/path", entries[0].Path)
	}
	if entries[1].Path != "/another/good" {
		t.Errorf("second entry path=%q, want /another/good", entries[1].Path)
	}
}

func TestLoadZEntriesFromFile_TrailingNewline(t *testing.T) {
	dir := t.TempDir()
	zFile := filepath.Join(dir, ".z")
	content := "/Users/me/code|100|1774000000\n"
	if err := os.WriteFile(zFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	entries := loadZEntriesFromFile(zFile)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestLoadZEntriesFromFile_DuplicatePaths(t *testing.T) {
	dir := t.TempDir()
	zFile := filepath.Join(dir, ".z")
	content := strings.Join([]string{
		"/Users/me/code|100|1774000000",
		"/Users/me/code|200|1774001000",
	}, "\n")
	if err := os.WriteFile(zFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	entries := loadZEntriesFromFile(zFile)
	// z file can have duplicate paths; loadZEntriesFromFile doesn't deduplicate
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (no dedup), got %d", len(entries))
	}
}

func TestFilterZSuggestions_PrefixMatch(t *testing.T) {
	entries := []zEntry{
		{Path: "/Users/me/code/skills", Rank: 200, Timestamp: 1774000000},
		{Path: "/Users/me/code/other", Rank: 50, Timestamp: 1773000000},
		{Path: "/tmp/scratch", Rank: 10, Timestamp: 1770000000},
	}

	results := filterZSuggestions("skills", entries, nil)
	if len(results) == 0 {
		t.Fatal("expected results matching 'skills'")
	}
	if !strings.Contains(results[0], "skills") {
		t.Errorf("first result should contain 'skills', got %q", results[0])
	}
}

func TestFilterZSuggestions_EmptyQuery(t *testing.T) {
	entries := []zEntry{
		{Path: "/Users/me/code/skills", Rank: 200, Timestamp: 1774000000},
		{Path: "/Users/me/code/other", Rank: 50, Timestamp: 1773000000},
	}

	results := filterZSuggestions("", entries, nil)
	// Empty query should return top entries by frecency
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for empty query, got %d", len(results))
	}
}

func TestFilterZSuggestions_RankedByFrecency(t *testing.T) {
	entries := []zEntry{
		{Path: "/Users/me/code/low", Rank: 10, Timestamp: 1770000000},
		{Path: "/Users/me/code/high", Rank: 200, Timestamp: 1774000000},
	}

	results := filterZSuggestions("code", entries, nil)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Higher frecency should come first
	if !strings.Contains(results[0], "high") {
		t.Errorf("expected higher frecency path first, got %q", results[0])
	}
}

func TestFilterZSuggestions_MaxFive(t *testing.T) {
	var entries []zEntry
	for i := 0; i < 20; i++ {
		entries = append(entries, zEntry{
			Path:      "/Users/me/code/repo" + string(rune('a'+i)),
			Rank:      float64(i * 10),
			Timestamp: int64(1774000000 + i),
		})
	}

	results := filterZSuggestions("code", entries, nil)
	if len(results) > 5 {
		t.Errorf("expected max 5 suggestions, got %d", len(results))
	}
}

func TestFilterZSuggestions_CaseInsensitive(t *testing.T) {
	entries := []zEntry{
		{Path: "/Users/me/Code/Skills", Rank: 100, Timestamp: 1774000000},
	}

	results := filterZSuggestions("skills", entries, nil)
	if len(results) == 0 {
		t.Fatal("expected case-insensitive match")
	}
}

func TestFilterZSuggestions_NoMatch(t *testing.T) {
	entries := []zEntry{
		{Path: "/Users/me/code/skills", Rank: 100, Timestamp: 1774000000},
	}

	results := filterZSuggestions("zzzzz", entries, nil)
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestFilterZSuggestions_ExcludesNonexistent(t *testing.T) {
	realDir := t.TempDir()
	entries := []zEntry{
		{Path: realDir, Rank: 100, Timestamp: 1774000000},
		{Path: "/nonexistent/fake/path", Rank: 200, Timestamp: 1774000000},
	}

	results := filterZSuggestions("", entries, dirExists)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}
	if results[0] != realDir {
		t.Errorf("expected %q, got %q", realDir, results[0])
	}
}

func TestFilterZSuggestions_ExcludesFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(file, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	entries := []zEntry{
		{Path: dir, Rank: 100, Timestamp: 1774000000},
		{Path: file, Rank: 200, Timestamp: 1774000000},
	}

	results := filterZSuggestions("", entries, dirExists)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (dir only), got %d: %v", len(results), results)
	}
	if results[0] != dir {
		t.Errorf("expected %q, got %q", dir, results[0])
	}
}

func TestFilterZSuggestions_AllStale(t *testing.T) {
	entries := []zEntry{
		{Path: "/nonexistent/path/a", Rank: 100, Timestamp: 1774000000},
		{Path: "/nonexistent/path/b", Rank: 200, Timestamp: 1774000000},
	}

	results := filterZSuggestions("", entries, dirExists)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %v", len(results), results)
	}
}

func TestFilterZSuggestions_NilPathExistsAcceptsAll(t *testing.T) {
	entries := []zEntry{
		{Path: "/nonexistent/but/accepted", Rank: 100, Timestamp: 1774000000},
	}

	// nil pathExists should accept all paths (backwards compat)
	results := filterZSuggestions("", entries, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result with nil pathExists, got %d", len(results))
	}
}
