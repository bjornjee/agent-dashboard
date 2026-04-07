package tui

import (
	"bytes"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// renderDiagramsPanel returns the contents of the diagrams panel: a top
// "dropdown" list of all diagrams for the selected agent (latest first)
// followed by a chroma-highlighted preview of the selected diagram's source.
//
// This is a pure render method — it must not mutate model state. Cursor
// clamping happens in key handlers; the chroma preview cache is populated
// in the diagramsLoadedMsg / cursor-move handlers.
func (m model) renderDiagramsPanel() string {
	if len(m.diagrams) == 0 {
		return "  No diagrams captured for this session."
	}
	cursor := m.diagramsCursor
	if cursor >= len(m.diagrams) {
		cursor = len(m.diagrams) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	w := m.rightWidth - 4
	if w < 20 {
		w = 20
	}

	cursorStyle := lipgloss.NewStyle().Foreground(themeLavender).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(themeOverlay1)
	typeStyle := lipgloss.NewStyle().Foreground(themeSapphire)

	var lines []string
	for i, d := range m.diagrams {
		marker := "  "
		if i == cursor {
			marker = cursorStyle.Render("▸ ")
		}
		title := d.Title
		if title == "" {
			title = "(untitled)"
		}
		typeLabel := d.Type
		if typeLabel == "" {
			typeLabel = "diagram"
		}
		// Reserve space for the type label on the right.
		const typeWidth = 18
		titleWidth := w - 4 - typeWidth - 2
		if titleWidth < 8 {
			titleWidth = 8
		}
		if len([]rune(title)) > titleWidth {
			title = string([]rune(title)[:titleWidth-1]) + "\u2026"
		}
		// Pad title.
		pad := titleWidth - len([]rune(title))
		if pad < 0 {
			pad = 0
		}
		row := marker + title + strings.Repeat(" ", pad+2) + typeStyle.Render(typeLabel)
		if i == cursor {
			lines = append(lines, cursorStyle.Render(row))
		} else {
			lines = append(lines, row)
		}
	}

	separator := mutedStyle.Render(strings.Repeat("─", w))
	hint := mutedStyle.Render("  j/k select · ⏎ render · x delete · D close")

	// Render preview body of selected diagram. The cache is populated by
	// key handlers; on cache miss (first render) we fall back to highlighting
	// inline without persisting (no mutation in render path).
	preview := m.renderedDiagramSrc
	if preview == "" {
		preview = highlightMermaid(m.diagrams[cursor].Source)
	}

	parts := []string{
		hint,
		separator,
		strings.Join(lines, "\n"),
		separator,
		preview,
	}
	return strings.Join(parts, "\n")
}

// highlightMermaid runs the source through chroma using a generic text lexer
// (chroma v2 has no mermaid lexer) so it picks up keywords and strings via
// the catppuccin-frappe palette consistent with the rest of the dashboard.
func highlightMermaid(src string) string {
	lexer := lexers.Get("yaml")
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("catppuccin-frappe")
	if style == nil {
		style = styles.Get("monokai")
	}
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Get("terminal256")
	}
	if formatter == nil {
		return src
	}
	iter, err := lexer.Tokenise(nil, src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iter); err != nil {
		return src
	}
	return strings.TrimRight(buf.String(), "\n")
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
