package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunMissingSourceLeavesTargetsUntouched(t *testing.T) {
	dir := t.TempDir()
	claudeOut := filepath.Join(dir, "claude")
	codexOut := filepath.Join(dir, "codex")
	writeFile(t, filepath.Join(claudeOut, "keep.md"), "existing\n")
	writeFile(t, filepath.Join(codexOut, "keep.md"), "existing\n")

	err := run(filepath.Join(dir, "does-not-exist"), claudeOut, codexOut)
	if err == nil {
		t.Fatal("run() error = nil, want source-root error")
	}
	if !strings.Contains(err.Error(), "source root") {
		t.Fatalf("run() error = %q, want source-root context", err)
	}
	// The guard must fire before the destructive clean.
	if got := readFile(t, filepath.Join(claudeOut, "keep.md")); got != "existing\n" {
		t.Fatalf("claude target modified despite missing source: %q", got)
	}
	if got := readFile(t, filepath.Join(codexOut, "keep.md")); got != "existing\n" {
		t.Fatalf("codex target modified despite missing source: %q", got)
	}
}

func TestRunEmitsTransformedTargets(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	claudeOut := filepath.Join(dir, "claude")
	codexOut := filepath.Join(dir, "codex")

	skill := strings.Join([]string{
		"shared /agent-dashboard:pr",
		"<!-- claude-only -->",
		"claude line",
		"<!-- /claude-only -->",
		"<!-- codex-only -->",
		"codex line",
		"<!-- /codex-only -->",
		"",
	}, "\n")
	writeFile(t, filepath.Join(src, "demo", "SKILL.md"), skill)
	// Non-SKILL.md markdown must be transformed too (shared fragments carry markers).
	writeFile(t, filepath.Join(src, "_shared", "setup.md"), "<!-- codex-only -->\ncodex only\n<!-- /codex-only -->\n")
	// Non-markdown files pass through verbatim.
	writeFile(t, filepath.Join(src, "_shared", "data.txt"), "<!-- codex-only -->\nverbatim\n")
	// Stale target content must be cleaned, not merged.
	writeFile(t, filepath.Join(claudeOut, "stale", "SKILL.md"), "stale\n")

	if err := run(src, claudeOut, codexOut); err != nil {
		t.Fatalf("run() unexpected error: %v", err)
	}

	if got, want := readFile(t, filepath.Join(claudeOut, "demo", "SKILL.md")), "shared /agent-dashboard:pr\nclaude line\n"; got != want {
		t.Fatalf("claude SKILL.md = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(codexOut, "demo", "SKILL.md")), "shared $agent-dashboard:pr\ncodex line\n"; got != want {
		t.Fatalf("codex SKILL.md = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(claudeOut, "_shared", "setup.md")), ""; got != want {
		t.Fatalf("claude setup.md = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(codexOut, "_shared", "setup.md")), "codex only\n"; got != want {
		t.Fatalf("codex setup.md = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(codexOut, "_shared", "data.txt")), "<!-- codex-only -->\nverbatim\n"; got != want {
		t.Fatalf("codex data.txt = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(claudeOut, "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale target dir survived the clean: err=%v", err)
	}
}
