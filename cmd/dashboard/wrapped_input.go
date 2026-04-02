package main

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// softWrapRunes splits runes into rows of at most width runes each.
// Returns at least one row (empty if input is empty).
// The returned slices share the backing array of runes.
func softWrapRunes(runes []rune, width int) [][]rune {
	if width <= 0 || len(runes) <= width {
		return [][]rune{runes}
	}
	var rows [][]rune
	for i := 0; i < len(runes); i += width {
		end := i + width
		if end > len(runes) {
			end = len(runes)
		}
		rows = append(rows, runes[i:end])
	}
	return rows
}

// isSlashCommand checks whether runes[start] begins a /skill command.
// The slash must be at position 0 or preceded by a space.
// Returns the exclusive end index and whether a match was found.
func isSlashCommand(runes []rune, start int, skills []string) (int, bool) {
	if start >= len(runes) || runes[start] != '/' {
		return 0, false
	}
	if start > 0 && !unicode.IsSpace(runes[start-1]) {
		return 0, false
	}
	end := start + 1
	for end < len(runes) && isCommandChar(runes[end]) {
		end++
	}
	if end == start+1 {
		return 0, false // bare slash
	}
	word := string(runes[start+1 : end])
	for _, s := range skills {
		if s == word {
			return end, true
		}
	}
	return 0, false
}

// isCommandChar returns true for characters valid in a slash command name.
func isCommandChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

var (
	slashStyle       = lipgloss.NewStyle().Foreground(themeBlue)
	cursorStyle      = lipgloss.NewStyle().Reverse(true)
	cursorSlashStyle = lipgloss.NewStyle().Reverse(true).Foreground(themeBlue)
	placeholderStyle = lipgloss.NewStyle().Foreground(themeOverlay1)
)

// renderWrappedInput renders an input value with soft wrapping, a cursor, and
// slash-command highlighting. It replaces textinput.Model.View() for all input
// fields in the dashboard.
func renderWrappedInput(value string, cursorPos int, width int, focused bool, skills []string, indent ...string) string {
	prefix := ""
	if len(indent) > 0 {
		prefix = indent[0]
	}

	if width < 1 {
		width = 1
	}

	runes := []rune(value)

	// Empty input: show cursor block when focused, placeholder otherwise.
	if len(runes) == 0 {
		if focused {
			return cursorStyle.Render(" ")
		}
		return placeholderStyle.Render("Type here...")
	}

	// Build token map: false = normal, true = slash command.
	isSlash := make([]bool, len(runes))
	for i := 0; i < len(runes); i++ {
		if runes[i] == '/' {
			end, matched := isSlashCommand(runes, i, skills)
			if matched {
				for j := i; j < end; j++ {
					isSlash[j] = true
				}
				i = end - 1
			}
		}
	}

	rows := softWrapRunes(runes, width)

	// Map cursorPos to (row, col).
	cursorRow, cursorCol := -1, -1
	if focused {
		pos := clamp(cursorPos, 0, len(runes))
		if pos == len(runes) {
			lastRow := len(rows) - 1
			lastLen := len(rows[lastRow])
			if lastLen >= width {
				cursorRow = lastRow + 1
				cursorCol = 0
			} else {
				cursorRow = lastRow
				cursorCol = lastLen
			}
		} else {
			cursorRow = pos / width
			cursorCol = pos % width
		}
	}

	// Render rows using span-based rendering: accumulate contiguous runs of the
	// same token type and flush them as a single styled string.
	var lines []string
	globalIdx := 0
	for r, row := range rows {
		var b strings.Builder
		spanStart := 0
		spanIsSlash := false
		if len(row) > 0 {
			spanIsSlash = isSlash[globalIdx]
		}

		for c := 0; c <= len(row); c++ {
			atCursor := focused && r == cursorRow && c == cursorCol
			atEnd := c == len(row)
			tokenChanged := !atEnd && isSlash[globalIdx+c] != spanIsSlash

			if atCursor || tokenChanged || atEnd {
				// Flush span before cursor/boundary.
				if c > spanStart {
					span := string(row[spanStart:c])
					if spanIsSlash {
						b.WriteString(slashStyle.Render(span))
					} else {
						b.WriteString(span)
					}
				}
				if atCursor {
					// Render cursor character.
					if atEnd {
						b.WriteString(cursorStyle.Render(" "))
					} else {
						ch := string(row[c])
						if isSlash[globalIdx+c] {
							b.WriteString(cursorSlashStyle.Render(ch))
						} else {
							b.WriteString(cursorStyle.Render(ch))
						}
					}
					spanStart = c + 1
					if spanStart < len(row) {
						spanIsSlash = isSlash[globalIdx+spanStart]
					}
				} else if tokenChanged {
					spanStart = c
					spanIsSlash = isSlash[globalIdx+c]
				}
			}
		}

		globalIdx += len(row)
		lines = append(lines, b.String())
	}

	// Cursor on a new row past the last (wrap at exact boundary).
	if cursorRow == len(rows) {
		lines = append(lines, cursorStyle.Render(" "))
	}

	if prefix != "" {
		for i := 1; i < len(lines); i++ {
			lines[i] = prefix + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
