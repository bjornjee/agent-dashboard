package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseZLine(t *testing.T) {
	tests := []struct {
		line     string
		wantOk   bool
		wantPath string
		wantRank float64
	}{
		{"/Users/me/code|100|1774000000", true, "/Users/me/code", 100},
		{"/tmp/test|0.5|1770000000", true, "/tmp/test", 0.5},
		{"invalid line", false, "", 0},
		{"", false, "", 0},
		{"|100|123", false, "", 0}, // empty path
	}

	for _, tt := range tests {
		entry, ok := parseZLine(tt.line)
		if ok != tt.wantOk {
			t.Errorf("parseZLine(%q): ok=%v, want %v", tt.line, ok, tt.wantOk)
			continue
		}
		if ok {
			if entry.Path != tt.wantPath {
				t.Errorf("parseZLine(%q): path=%q, want %q", tt.line, entry.Path, tt.wantPath)
			}
			if entry.Rank != tt.wantRank {
				t.Errorf("parseZLine(%q): rank=%f, want %f", tt.line, entry.Rank, tt.wantRank)
			}
		}
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
