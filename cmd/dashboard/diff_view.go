package main

import (
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddStyle    = lipgloss.NewStyle().Foreground(themeGreen)
	diffDelStyle    = lipgloss.NewStyle().Foreground(themeRed)
	diffHunkStyle   = lipgloss.NewStyle().Foreground(themeMauve).Bold(true)
	diffFileModIcon = lipgloss.NewStyle().Foreground(themeYellow).Render("~")
	diffFileAddIcon = lipgloss.NewStyle().Foreground(themeGreen).Render("+")
	diffFileDelIcon = lipgloss.NewStyle().Foreground(themeRed).Render("-")
)

func (m model) diffFileTreeContent() string {
	var lines []string
	for i, f := range m.diffFiles {
		icon := diffFileModIcon
		name := f.NewName
		if f.IsNew {
			icon = diffFileAddIcon
			name = f.NewName
		} else if f.IsDelete {
			icon = diffFileDelIcon
			name = f.OldName
		}

		line := fmt.Sprintf(" %s %s", icon, name)
		if i == m.selectedDiffFile {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m model) diffSideBySideContent() string {
	if m.selectedDiffFile >= len(m.diffFiles) {
		return ""
	}
	file := m.diffFiles[m.selectedDiffFile]

	if file.IsBinary {
		return helpStyle.Render("  Binary file")
	}
	if len(file.TextFragments) == 0 {
		return helpStyle.Render("  No text changes (mode change only)")
	}

	halfWidth := (m.rightWidth - 3) / 2 // -3 for "│" separator + padding
	if halfWidth < 10 {
		halfWidth = 10
	}
	lineNumWidth := 4

	var rows []string

	// Header
	oldName := file.OldName
	newName := file.NewName
	if oldName == "" {
		oldName = "/dev/null"
	}
	if newName == "" {
		newName = "/dev/null"
	}
	rows = append(rows, fmt.Sprintf(" %s  │  %s",
		boldStyle.Render(truncOrPad(oldName, halfWidth-1)),
		boldStyle.Render(truncOrPad(newName, halfWidth-1)),
	))
	rows = append(rows, helpStyle.Render(strings.Repeat("─", halfWidth))+"─┼─"+helpStyle.Render(strings.Repeat("─", halfWidth)))

	for _, frag := range file.TextFragments {
		// Hunk header
		hunkHeader := diffHunkStyle.Render(fmt.Sprintf(" @@ -%d,%d +%d,%d @@",
			frag.OldPosition, frag.OldLines, frag.NewPosition, frag.NewLines))
		rows = append(rows, hunkHeader)

		oldLineNum := int(frag.OldPosition)
		newLineNum := int(frag.NewPosition)

		contentWidth := halfWidth - lineNumWidth - 2 // -2 for spacing

		for _, line := range frag.Lines {
			content := strings.TrimRight(line.Line, "\n\r")

			switch line.Op {
			case gitdiff.OpContext:
				left := formatDiffLine(oldLineNum, content, contentWidth, lineNumWidth, helpStyle)
				right := formatDiffLine(newLineNum, content, contentWidth, lineNumWidth, helpStyle)
				rows = append(rows, left+" │ "+right)
				oldLineNum++
				newLineNum++

			case gitdiff.OpDelete:
				left := formatDiffLine(oldLineNum, content, contentWidth, lineNumWidth, diffDelStyle)
				right := strings.Repeat(" ", halfWidth)
				rows = append(rows, left+" │ "+right)
				oldLineNum++

			case gitdiff.OpAdd:
				left := strings.Repeat(" ", halfWidth)
				right := formatDiffLine(newLineNum, content, contentWidth, lineNumWidth, diffAddStyle)
				rows = append(rows, left+" │ "+right)
				newLineNum++
			}
		}
	}

	return strings.Join(rows, "\n")
}

func formatDiffLine(lineNum int, content string, contentWidth, lineNumWidth int, style lipgloss.Style) string {
	numStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)
	truncated := truncOrPad(content, contentWidth)
	return style.Render(fmt.Sprintf(" %s %s", numStr, truncated))
}

func truncOrPad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > width {
		if width > 1 {
			return string(runes[:width-1]) + "…"
		}
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

func (m model) renderDiffFilePanel() string {
	panelHeight := m.height - 5 - bannerHeight
	header := titleStyle.Render(" FILES CHANGED")
	content := header + "\n\n" + m.diffFileVP.View()
	return borderStyle.
		Width(m.leftWidth).
		Height(panelHeight).
		Render(content)
}

func (m model) renderDiffContentPanel() string {
	panelHeight := m.height - 5 - bannerHeight
	header := titleStyle.Render(" DIFF")
	content := header + "\n\n" + m.diffContentVP.View()
	return borderStyle.
		Width(m.rightWidth).
		Height(panelHeight).
		Render(content)
}

func (m *model) updateDiffContent() {
	m.diffFileVP.SetContent(m.diffFileTreeContent())
	m.diffContentVP.SetContent(m.diffSideBySideContent())
}
