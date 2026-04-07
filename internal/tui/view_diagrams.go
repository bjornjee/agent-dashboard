package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderDiagramsPanel returns the contents of the diagrams panel: a top
// list of all diagrams for the selected agent (latest first) followed by a
// custom-highlighted preview of the selected diagram's source.
//
// This is a pure render method — it must not mutate model state. Cursor
// clamping happens in key handlers; the highlighted-source cache is
// populated in the diagramsLoadedMsg / cursor-move handlers.
func (m model) renderDiagramsPanel() string {
	if len(m.diagrams) == 0 {
		return mutedStyle().Render("  No diagrams captured for this session.")
	}
	cursor := m.diagramsCursor
	if cursor >= len(m.diagrams) {
		cursor = len(m.diagrams) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	w := m.rightWidth - 4
	if w < 24 {
		w = 24
	}

	var (
		hintStyle      = lipgloss.NewStyle().Foreground(themeOverlay1)
		separatorStyle = lipgloss.NewStyle().Foreground(themeSurface2)
		rowStyle       = lipgloss.NewStyle().Foreground(themeText)
		dimStyle       = lipgloss.NewStyle().Foreground(themeOverlay1)
		selectedStyle  = lipgloss.NewStyle().Foreground(themeBase).Background(themeLavender).Bold(true)
		typeStyle      = lipgloss.NewStyle().Foreground(themeOverlay2)
	)

	// List rows. Two-column layout: title (flex), type (right-aligned).
	const typeWidth = 16
	titleWidth := w - 4 - typeWidth - 2 // 4 for marker+gutter, 2 spacer
	if titleWidth < 12 {
		titleWidth = 12
	}

	var lines []string
	for i, d := range m.diagrams {
		title := d.Title
		if title == "" {
			title = "(untitled)"
		}
		if runeLen(title) > titleWidth {
			title = truncRunes(title, titleWidth-1) + "\u2026"
		}
		title = padRight(title, titleWidth)

		typeLabel := d.Type
		if typeLabel == "" {
			typeLabel = "diagram"
		}
		if runeLen(typeLabel) > typeWidth {
			typeLabel = truncRunes(typeLabel, typeWidth-1) + "\u2026"
		}
		typeLabel = padRight(typeLabel, typeWidth)

		if i == cursor {
			row := "  " + title + "  " + typeLabel
			// Pad to full width so the highlight bar fills the row.
			if pad := w - runeLen(row); pad > 0 {
				row += strings.Repeat(" ", pad)
			}
			lines = append(lines, selectedStyle.Render(row))
		} else {
			row := "  " + rowStyle.Render(title) + "  " + typeStyle.Render(typeLabel)
			lines = append(lines, row)
		}
	}

	separator := separatorStyle.Render(strings.Repeat("─", w))
	hint := hintStyle.Render("  j/k select · ⏎ open in browser · x delete · D close")
	count := dimStyle.Render("  " + plural(len(m.diagrams), "diagram", "diagrams"))

	preview := m.renderedDiagramSrc
	if preview == "" {
		preview = highlightMermaid(m.diagrams[cursor].Source)
	}

	parts := []string{
		hint,
		count,
		separator,
		strings.Join(lines, "\n"),
		separator,
		preview,
	}
	return strings.Join(parts, "\n")
}

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themeOverlay1)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func runeLen(s string) int { return len([]rune(s)) }

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func padRight(s string, w int) string {
	pad := w - runeLen(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

// mermaidKeywords are the diagram-type tokens that appear at the start of a
// mermaid source. Highlighted in the accent colour at the top of the body.
var mermaidKeywords = []string{
	"flowchart", "graph", "sequenceDiagram", "stateDiagram-v2", "stateDiagram",
	"classDiagram", "erDiagram", "gantt", "pie", "journey", "gitGraph",
	"mindmap", "timeline", "quadrantChart", "requirementDiagram", "C4Context",
	"subgraph", "end",
}

// arrowTokens are the connector glyphs we accent. Order matters — longer
// patterns must come first so we don't half-match.
var arrowTokens = []string{
	"<-->", "<-.->", "==>", "<==", "-.->", "-->", "->>", "-->>",
	"--x", "--o", "<--", "..>", "-.->", "-->|", "==", "->",
	"--", "..", "::",
}

// highlightMermaid renders a mermaid source with a small, hand-rolled
// highlighter tuned for readability over fidelity. Comments are dimmed,
// diagram-type keywords and `subgraph`/`end` are accented, arrows are
// painted in a warm accent, and string literals get a subtle hue. The
// fallback colour is plain `themeText` so the bulk of the diagram reads
// as ordinary monospace text on the dashboard background.
func highlightMermaid(src string) string {
	var (
		comment = lipgloss.NewStyle().Foreground(themeOverlay0).Italic(true)
		keyword = lipgloss.NewStyle().Foreground(themeMauve).Bold(true)
		arrow   = lipgloss.NewStyle().Foreground(themePeach)
		str     = lipgloss.NewStyle().Foreground(themeGreen)
		text    = lipgloss.NewStyle().Foreground(themeText)
	)
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(trimmed, "%%") || trimmed == "---" {
			out = append(out, comment.Render(line))
			continue
		}
		out = append(out, highlightMermaidLine(line, keyword, arrow, str, text))
	}
	return strings.Join(out, "\n")
}

func highlightMermaidLine(line string, keyword, arrow, str, text lipgloss.Style) string {
	var b strings.Builder
	i := 0
	for i < len(line) {
		ch := line[i]
		// String literal in double quotes.
		if ch == '"' {
			j := i + 1
			for j < len(line) && line[j] != '"' {
				j++
			}
			if j < len(line) {
				j++
			}
			b.WriteString(str.Render(line[i:j]))
			i = j
			continue
		}
		// Arrow tokens (longest match first).
		matched := false
		for _, tok := range arrowTokens {
			if strings.HasPrefix(line[i:], tok) {
				b.WriteString(arrow.Render(tok))
				i += len(tok)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		// Identifier / keyword.
		if isIdentStart(ch) {
			j := i
			for j < len(line) && isIdentCont(line[j]) {
				j++
			}
			word := line[i:j]
			if isMermaidKeyword(word) {
				b.WriteString(keyword.Render(word))
			} else {
				b.WriteString(text.Render(word))
			}
			i = j
			continue
		}
		b.WriteString(text.Render(string(ch)))
		i++
	}
	return b.String()
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}

func isMermaidKeyword(w string) bool {
	for _, k := range mermaidKeywords {
		if w == k {
			return true
		}
	}
	return false
}

// diagramBadge returns the rendered badge string for an agent's diagram
// indicator (📑), with brighter color when the count exceeds the user's
// last-seen count for this session, or empty if the agent has no diagrams.
func (m model) diagramBadge(sessionID string, count int) string {
	if count <= 0 {
		return ""
	}
	seen := m.lastSeenDiagramCount[sessionID]
	if count > seen {
		return lipgloss.NewStyle().Foreground(themeLavender).Bold(true).Render("📑")
	}
	return lipgloss.NewStyle().Foreground(themeOverlay1).Render("📑")
}
