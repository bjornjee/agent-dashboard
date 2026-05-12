// Package spawnutil provides shell-quoting and prompt-building helpers shared
// across harness sub-packages. It is a leaf package — no other harness package
// may depend on it to avoid import cycles.
package spawnutil

import "strings"

// BuildPrompt returns "/skill message", "/skill", "message", or "" depending
// on which of the two inputs are non-empty.
func BuildPrompt(skill, message string) string {
	var parts []string
	if skill != "" {
		parts = append(parts, "/"+skill)
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

// ShellQuote wraps s in single quotes and escapes any embedded single quotes
// so the result is safe to pass as a shell word (e.g. inside tmux send-keys).
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
