package main

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"charm.land/lipgloss/v2"
)

// Diff background RGB values (subtle tints on Catppuccin Frappé base #303446).
const (
	diffAddBgR, diffAddBgG, diffAddBgB = 40, 56, 40 // greenish tint
	diffDelBgR, diffDelBgG, diffDelBgB = 56, 36, 36 // reddish tint
)

// collapsibleThreshold is the minimum number of consecutive context lines
// before the middle is collapsed.
const collapsibleThreshold = 5

// contextKeep is how many context lines to keep at the edges of a collapsed block.
const contextKeep = 2

var (
	diffAddStyle    = lipgloss.NewStyle().Foreground(themeGreen)
	diffDelStyle    = lipgloss.NewStyle().Foreground(themeRed)
	diffHunkStyle   = lipgloss.NewStyle().Foreground(themeMauve).Bold(true)
	diffFileModIcon = lipgloss.NewStyle().Foreground(themeYellow).Render("~")
	diffFileAddIcon = lipgloss.NewStyle().Foreground(themeGreen).Render("+")
	diffFileDelIcon = lipgloss.NewStyle().Foreground(themeRed).Render("-")
	diffDirStyle    = lipgloss.NewStyle().Foreground(themeOverlay1)
	diffCountStyle  = lipgloss.NewStyle().Foreground(themeOverlay1)
)

func diffFileName(f *gitdiff.File) string {
	if f.IsDelete {
		return f.OldName
	}
	return f.NewName
}

// diffTreeEntry represents a single row in the tree-view file list.
type diffTreeEntry struct {
	isDir   bool   // true for directory headers
	label   string // display label (dir path with "/" or basename)
	fileIdx int    // index into diffFiles; -1 for directory headers
	indent  int    // indentation level (0=root, 1=under directory)
	dirKey  string // unique dir path for collapse tracking (dirs only)
	hidden  bool   // true when parent is collapsed or filtered out
}

// buildDiffTreeEntries groups diffFiles by directory into an ordered tree.
func (m *model) buildDiffTreeEntries() {
	type dirGroup struct {
		dir     string
		indices []int
	}

	var rootIndices []int
	var dirs []dirGroup
	dirIndex := map[string]int{} // dir -> index in dirs slice

	for i, f := range m.diffFiles {
		name := diffFileName(f)
		dir := path.Dir(name)
		if dir == "." {
			rootIndices = append(rootIndices, i)
			continue
		}
		if idx, ok := dirIndex[dir]; ok {
			dirs[idx].indices = append(dirs[idx].indices, i)
		} else {
			dirIndex[dir] = len(dirs)
			dirs = append(dirs, dirGroup{dir: dir, indices: []int{i}})
		}
	}

	m.diffTreeEntries = nil

	// Root files first (no directory header)
	for _, i := range rootIndices {
		name := diffFileName(m.diffFiles[i])
		m.diffTreeEntries = append(m.diffTreeEntries, diffTreeEntry{
			label:   name,
			fileIdx: i,
		})
	}

	// Then each directory group
	for _, dg := range dirs {
		dk := dg.dir + "/"
		m.diffTreeEntries = append(m.diffTreeEntries, diffTreeEntry{
			isDir:   true,
			label:   dk,
			fileIdx: -1,
			dirKey:  dk,
		})
		for _, i := range dg.indices {
			name := diffFileName(m.diffFiles[i])
			m.diffTreeEntries = append(m.diffTreeEntries, diffTreeEntry{
				label:   path.Base(name),
				fileIdx: i,
				indent:  1,
				dirKey:  dk,
			})
		}
	}
	m.applyTreeVisibility()
}

// applyTreeVisibility sets hidden on entries based on collapsed dirs and filter.
func (m *model) applyTreeVisibility() {
	filter := strings.ToLower(m.diffFilterText)

	for i := range m.diffTreeEntries {
		e := &m.diffTreeEntries[i]
		e.hidden = false

		if e.isDir {
			// If filter is active, hide dirs with no matching children
			if filter != "" {
				hasMatch := false
				for j := i + 1; j < len(m.diffTreeEntries); j++ {
					child := m.diffTreeEntries[j]
					if child.isDir {
						break
					}
					fullName := strings.ToLower(diffFileName(m.diffFiles[child.fileIdx]))
					if strings.Contains(fullName, filter) {
						hasMatch = true
						break
					}
				}
				e.hidden = !hasMatch
			}
			continue
		}

		// File entry
		// Check filter
		if filter != "" {
			fullName := strings.ToLower(diffFileName(m.diffFiles[e.fileIdx]))
			if !strings.Contains(fullName, filter) {
				e.hidden = true
				continue
			}
		}

		// Check if parent dir is collapsed
		if e.dirKey != "" && m.diffCollapsedDirs[e.dirKey] {
			e.hidden = true
		}
	}
}

// visibleDiffEntries returns indices into diffTreeEntries that are not hidden.
func (m model) visibleDiffEntries() []int {
	var vis []int
	for i, e := range m.diffTreeEntries {
		if !e.hidden {
			vis = append(vis, i)
		}
	}
	return vis
}

// moveDiffCursor moves the cursor by delta within visible entries.
func (m *model) moveDiffCursor(delta int) {
	vis := m.visibleDiffEntries()
	if len(vis) == 0 {
		return
	}

	newCursor := m.diffCursor + delta
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= len(vis) {
		newCursor = len(vis) - 1
	}
	m.diffCursor = newCursor

	// If landed on a file, update selected file
	entry := m.diffTreeEntries[vis[newCursor]]
	if !entry.isDir {
		m.selectedDiffFile = entry.fileIdx
		m.diffExpandedAll = false
	}
	m.updateDiffContent()
}

// toggleDiffDir toggles collapse for the directory under cursor.
func (m *model) toggleDiffDir() {
	vis := m.visibleDiffEntries()
	if m.diffCursor >= len(vis) {
		return
	}
	idx := vis[m.diffCursor]
	entry := m.diffTreeEntries[idx]
	if !entry.isDir {
		return
	}
	m.diffCollapsedDirs[entry.dirKey] = !m.diffCollapsedDirs[entry.dirKey]
	m.applyTreeVisibility()
	// Clamp cursor if visible list shrank
	vis = m.visibleDiffEntries()
	if len(vis) > 0 && m.diffCursor >= len(vis) {
		m.diffCursor = len(vis) - 1
	}
	m.updateDiffContent()
}

// dirFileCount returns the number of (non-hidden) file children for a dir entry.
func (m model) dirFileCount(dirKey string) int {
	count := 0
	for _, e := range m.diffTreeEntries {
		if !e.isDir && e.dirKey == dirKey && !e.hidden {
			count++
		}
	}
	return count
}

// -- Syntax highlighting --

// syntaxHighlightLine applies chroma syntax highlighting to a single line of code.
func syntaxHighlightLine(lexer chroma.Lexer, content string) string {
	if lexer == nil {
		return content
	}

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
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
		return content
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return content
	}

	// Chroma may add a trailing newline; strip it
	result := strings.TrimRight(buf.String(), "\n")
	return result
}

// getLexerForFile returns a chroma lexer for the given filename, or nil.
func getLexerForFile(filename string) chroma.Lexer {
	lexer := lexers.Match(filename)
	if lexer == nil {
		return nil
	}
	return chroma.Coalesce(lexer)
}

// -- Background overlay --

// applyDiffBackground applies an RGB background color to a line, preserving
// inner ANSI foreground colors. Pads to the given width.
func applyDiffBackground(line string, r, g, b, width int) string {
	bgCode := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	const reset = "\x1b[0m"

	// Strip any trailing reset before processing so we don't get stale sequences.
	core := strings.TrimSuffix(line, reset)

	// Re-apply background after each inner reset so it persists through
	// chroma's foreground color codes.
	inner := strings.ReplaceAll(core, reset, reset+bgCode)

	// Pad to exact visible width so background fills the column.
	visWidth := lipgloss.Width(core)
	padding := ""
	if visWidth < width {
		padding = strings.Repeat(" ", width-visWidth)
	}

	return bgCode + inner + padding + reset
}

// -- ANSI-aware wrapping --

// wrapANSI splits a string that may contain ANSI escapes into lines of at most
// `width` visible characters. ANSI state is carried across line breaks so colors
// are preserved on continuation lines.
func wrapANSI(s string, width int) []string {
	if width <= 0 {
		return []string{""}
	}

	visWidth := lipgloss.Width(s)
	if visWidth <= width {
		padded := s + strings.Repeat(" ", width-visWidth)
		return []string{padded}
	}

	var lines []string
	var cur strings.Builder
	vis := 0
	inEscape := false
	// Track active ANSI codes so we can re-apply them on new lines
	var activeEscapes []string
	var escBuf strings.Builder

	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			escBuf.Reset()
			escBuf.WriteRune(r)
			continue
		}
		if inEscape {
			escBuf.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
				esc := escBuf.String()
				cur.WriteString(esc)
				// Track the escape: reset clears all, otherwise accumulate
				if esc == "\x1b[0m" {
					activeEscapes = activeEscapes[:0]
				} else {
					activeEscapes = append(activeEscapes, esc)
				}
			}
			continue
		}

		if vis >= width {
			// End current line, start new one
			cur.WriteString("\x1b[0m")
			line := cur.String()
			curVis := lipgloss.Width(line)
			if curVis < width {
				line += strings.Repeat(" ", width-curVis)
			}
			lines = append(lines, line)
			cur.Reset()
			vis = 0
			// Re-apply active escapes on the new line
			for _, e := range activeEscapes {
				cur.WriteString(e)
			}
		}

		cur.WriteRune(r)
		vis++
	}

	// Flush remaining
	if cur.Len() > 0 {
		cur.WriteString("\x1b[0m")
		line := cur.String()
		curVis := lipgloss.Width(line)
		if curVis < width {
			line += strings.Repeat(" ", width-curVis)
		}
		lines = append(lines, line)
	}

	return lines
}

// -- File tree --

// diffTreeCursorLine is set by diffFileTreeContent to the first rendered line
// index of the cursor entry (used for auto-scrolling).
func (m model) diffFileTreeContent() (string, int) {
	maxWidth := m.diffLeftWidth - 4
	if maxWidth < 10 {
		maxWidth = 10
	}

	vis := m.visibleDiffEntries()
	var lines []string
	cursorLine := 0

	for visIdx, entryIdx := range vis {
		entry := m.diffTreeEntries[entryIdx]
		isCursor := visIdx == m.diffCursor

		if isCursor {
			cursorLine = len(lines)
		}

		if entry.isDir {
			chevron := "▾"
			if m.diffCollapsedDirs[entry.dirKey] {
				chevron = "▸"
			}
			count := m.dirFileCount(entry.dirKey)
			countStr := diffCountStyle.Render(fmt.Sprintf(" (%d)", count))

			// Smart-truncate long directory paths from the left.
			dirLabel := entry.label
			dirMaxWidth := maxWidth - 5 - lipgloss.Width(countStr) // " ▾ " prefix + count
			if dirMaxWidth > 0 && len([]rune(dirLabel)) > dirMaxWidth {
				// Truncate from the left, keeping the rightmost (most unique) part.
				runes := []rune(dirLabel)
				dirLabel = "…" + string(runes[len(runes)-dirMaxWidth+1:])
			}

			if isCursor {
				// Highlight only the directory label, keep chevron and count normal.
				line := " " + chevron + " " + selectedStyle.Render(dirLabel) + countStr
				lines = append(lines, line)
			} else {
				line := " " + chevron + " " + diffDirStyle.Render(dirLabel) + countStr
				lines = append(lines, line)
			}
			continue
		}

		f := m.diffFiles[entry.fileIdx]
		icon := diffFileModIcon
		if f.IsNew {
			icon = diffFileAddIcon
		} else if f.IsDelete {
			icon = diffFileDelIcon
		}

		indentStr := strings.Repeat("   ", entry.indent)
		prefix := fmt.Sprintf("%s %s ", indentStr, icon)
		prefixWidth := lipgloss.Width(prefix)
		nameWidth := maxWidth - prefixWidth

		if len([]rune(entry.label)) > nameWidth && nameWidth > 0 {
			runes := []rune(entry.label)
			first := true
			for len(runes) > 0 {
				w := nameWidth
				if w > len(runes) {
					w = len(runes)
				}
				chunk := string(runes[:w])
				runes = runes[w:]

				var line string
				if first {
					if isCursor {
						line = prefix + selectedStyle.Render(chunk)
					} else {
						line = prefix + chunk
					}
					first = false
				} else {
					if isCursor {
						line = strings.Repeat(" ", prefixWidth) + selectedStyle.Render(chunk)
					} else {
						line = strings.Repeat(" ", prefixWidth) + chunk
					}
				}
				lines = append(lines, line)
			}
		} else {
			if isCursor {
				line := prefix + selectedStyle.Render(entry.label)
				lines = append(lines, line)
			} else {
				line := prefix + entry.label
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n"), cursorLine
}

// -- Side-by-side content with syntax highlighting & collapsible blocks --

func (m model) diffSideBySideContent() (string, []string) {
	if m.selectedDiffFile < 0 || m.selectedDiffFile >= len(m.diffFiles) {
		return "", nil
	}
	file := m.diffFiles[m.selectedDiffFile]

	if file.IsBinary {
		return helpStyle.Render("  Binary file"), nil
	}
	if len(file.TextFragments) == 0 {
		return helpStyle.Render("  No text changes (mode change only)"), nil
	}

	halfWidth := (m.diffRightWidth - 3) / 2 // -3 for "│" separator + padding
	if halfWidth < 10 {
		halfWidth = 10
	}
	lineNumWidth := 4
	contentWidth := halfWidth - lineNumWidth - 2 // -2 for spacing

	// Get chroma lexers for old and new files
	oldLexer := getLexerForFile(file.OldName)
	newLexer := getLexerForFile(file.NewName)

	var rows []string
	var funcCtx []string // parallel to rows: function context for each row

	// Helper to append rows with a given function context
	appendRows := func(newRows []string, ctx string) {
		rows = append(rows, newRows...)
		for range newRows {
			funcCtx = append(funcCtx, ctx)
		}
	}
	appendRow := func(row string, ctx string) {
		rows = append(rows, row)
		funcCtx = append(funcCtx, ctx)
	}

	// Header
	oldName := file.OldName
	newName := file.NewName
	if oldName == "" {
		oldName = "/dev/null"
	}
	if newName == "" {
		newName = "/dev/null"
	}
	appendRow(" "+boldStyle.Render(truncOrPad(oldName, halfWidth-1))+" │ "+boldStyle.Render(truncOrPad(newName, halfWidth-1)), "")
	appendRow(strings.Repeat("─", halfWidth)+"─┼─"+strings.Repeat("─", halfWidth), "")

	for _, frag := range file.TextFragments {
		// Function context from the hunk header (text after @@)
		curFuncCtx := strings.TrimSpace(frag.Comment)

		// Hunk header
		hunkHeader := diffHunkStyle.Render(fmt.Sprintf(" @@ -%d,%d +%d,%d @@",
			frag.OldPosition, frag.OldLines, frag.NewPosition, frag.NewLines))
		if curFuncCtx != "" {
			hunkHeader += " " + diffHunkStyle.Render(curFuncCtx)
		}
		appendRow(hunkHeader, curFuncCtx)

		// Collect lines and identify context runs for collapsing
		type diffLine struct {
			op      gitdiff.LineOp
			content string
		}
		var allLines []diffLine
		for _, line := range frag.Lines {
			allLines = append(allLines, diffLine{
				op:      line.Op,
				content: strings.TrimRight(line.Line, "\n\r"),
			})
		}

		// Identify context runs and potentially collapse them
		oldLineNum := int(frag.OldPosition)
		newLineNum := int(frag.NewPosition)

		i := 0
		for i < len(allLines) {
			dl := allLines[i]

			switch dl.op {
			case gitdiff.OpContext:
				// Find the run of consecutive context lines
				runStart := i
				for i < len(allLines) && allLines[i].op == gitdiff.OpContext {
					i++
				}
				runEnd := i
				runLen := runEnd - runStart

				if !m.diffExpandedAll && runLen > collapsibleThreshold {
					// Render first contextKeep lines
					for j := runStart; j < runStart+contextKeep && j < runEnd; j++ {
						left := formatDiffLineHighlighted(oldLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, oldLexer, 0, 0, 0)
						right := formatDiffLineHighlighted(newLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, newLexer, 0, 0, 0)
						appendRows(joinSideBySide(left, right, halfWidth), curFuncCtx)
						oldLineNum++
						newLineNum++
					}

					// Collapsed placeholder
					hidden := runLen - 2*contextKeep
					placeholder := fmt.Sprintf("··· %d lines hidden (press e to expand) ···", hidden)
					placeholderStyled := diffCollapsedStyle.Render(placeholder)
					leftPad := (halfWidth - lipgloss.Width(placeholder)) / 2
					if leftPad < 0 {
						leftPad = 0
					}
					paddedPlaceholder := strings.Repeat(" ", leftPad) + placeholderStyled
					appendRow(paddedPlaceholder+" │ "+paddedPlaceholder, curFuncCtx)
					oldLineNum += hidden
					newLineNum += hidden

					// Render last contextKeep lines
					for j := runEnd - contextKeep; j < runEnd; j++ {
						left := formatDiffLineHighlighted(oldLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, oldLexer, 0, 0, 0)
						right := formatDiffLineHighlighted(newLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, newLexer, 0, 0, 0)
						appendRows(joinSideBySide(left, right, halfWidth), curFuncCtx)
						oldLineNum++
						newLineNum++
					}
				} else {
					// Render all context lines normally
					for j := runStart; j < runEnd; j++ {
						left := formatDiffLineHighlighted(oldLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, oldLexer, 0, 0, 0)
						right := formatDiffLineHighlighted(newLineNum, allLines[j].content, contentWidth, lineNumWidth, halfWidth, newLexer, 0, 0, 0)
						appendRows(joinSideBySide(left, right, halfWidth), curFuncCtx)
						oldLineNum++
						newLineNum++
					}
				}

			case gitdiff.OpDelete:
				left := formatDiffLineHighlighted(oldLineNum, dl.content, contentWidth, lineNumWidth, halfWidth, oldLexer, diffDelBgR, diffDelBgG, diffDelBgB)
				right := []string{emptyHalf(halfWidth)}
				appendRows(joinSideBySide(left, right, halfWidth), curFuncCtx)
				oldLineNum++
				i++

			case gitdiff.OpAdd:
				left := []string{emptyHalf(halfWidth)}
				right := formatDiffLineHighlighted(newLineNum, dl.content, contentWidth, lineNumWidth, halfWidth, newLexer, diffAddBgR, diffAddBgG, diffAddBgB)
				appendRows(joinSideBySide(left, right, halfWidth), curFuncCtx)
				newLineNum++
				i++
			}
		}
	}

	return strings.Join(rows, "\n"), funcCtx
}

// tabWidth is the number of spaces used to replace tab characters.
const tabWidth = 4

// formatDiffLineHighlighted renders a diff line with syntax highlighting and
// optional background color, wrapping long lines. Returns one or more rows,
// each exactly halfWidth visible characters wide.
func formatDiffLineHighlighted(lineNum int, content string, contentWidth, lineNumWidth, halfWidth int, lexer chroma.Lexer, bgR, bgG, bgB int) []string {
	// Expand tabs to spaces before any processing.
	content = strings.ReplaceAll(content, "\t", strings.Repeat(" ", tabWidth))

	// Syntax-highlight the content
	highlighted := syntaxHighlightLine(lexer, content)

	// Wrap the highlighted content into chunks of contentWidth visible chars.
	wrappedContent := wrapANSI(highlighted, contentWidth)

	numPrefix := fmt.Sprintf(" %*d ", lineNumWidth, lineNum)
	blankPrefix := strings.Repeat(" ", lineNumWidth+2) // same width, no number

	hasBg := bgR != 0 || bgG != 0 || bgB != 0
	var rows []string

	for i, chunk := range wrappedContent {
		prefix := blankPrefix
		if i == 0 {
			prefix = numPrefix
		}

		line := prefix + chunk

		// Ensure exact halfWidth
		const reset = "\x1b[0m"
		lineCore := strings.TrimSuffix(line, reset)
		hadReset := len(lineCore) != len(line)

		visWidth := lipgloss.Width(lineCore)
		if visWidth < halfWidth {
			lineCore += strings.Repeat(" ", halfWidth-visWidth)
		}

		if hasBg {
			rows = append(rows, applyDiffBackground(lineCore, bgR, bgG, bgB, halfWidth))
		} else if hadReset {
			rows = append(rows, lineCore+reset)
		} else {
			rows = append(rows, lineCore)
		}
	}

	return rows
}

// emptyHalf returns a blank string of exactly halfWidth spaces.
func emptyHalf(halfWidth int) string {
	return strings.Repeat(" ", halfWidth)
}

// joinSideBySide pairs left and right row slices with the │ separator,
// padding the shorter side with blank rows.
func joinSideBySide(left, right []string, halfWidth int) []string {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	blank := emptyHalf(halfWidth)
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		l := blank
		r := blank
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows[i] = l + " │ " + r
	}
	return rows
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

// -- Panels --

func (m model) renderDiffFilePanel() string {
	panelHeight := m.height - 5 - m.bannerHeight()
	header := titleStyle.Render(" FILES CHANGED")

	filterLine := ""
	if m.diffFilterActive || m.diffFilterText != "" {
		filterLine = "\n " + renderWrappedInput(m.diffFilterInput.Value(), m.diffFilterInput.Position(), m.diffLeftWidth-4, m.diffFilterActive, nil, " ")
	}

	content := header + filterLine + "\n\n" + m.diffFileVP.View()
	return borderStyle.
		Width(m.diffLeftWidth).
		Height(panelHeight).
		Render(content)
}

func (m model) renderDiffContentPanel() string {
	panelHeight := m.height - 5 - m.bannerHeight()
	header := titleStyle.Render(" DIFF")

	// Sticky function context: find the function for the top visible row
	stickyCtx := ""
	if len(m.diffFuncCtx) > 0 {
		topRow := m.diffContentVP.YOffset()
		if topRow >= len(m.diffFuncCtx) {
			topRow = len(m.diffFuncCtx) - 1
		}
		if topRow >= 0 {
			// Walk backward from topRow to find the most recent non-empty context
			for i := topRow; i >= 0; i-- {
				if m.diffFuncCtx[i] != "" {
					stickyCtx = m.diffFuncCtx[i]
					break
				}
			}
		}
	}

	stickyLine := ""
	if stickyCtx != "" && !m.diffContentVP.AtTop() {
		stickyLine = "\n" + diffHunkStyle.Render(" "+stickyCtx)
	}

	// Scroll hints
	scrollUp := ""
	scrollDown := ""
	if !m.diffContentVP.AtTop() {
		scrollUp = "  " + diffScrollHintStyle.Render("▲ scroll up (ctrl+u)")
	}
	if !m.diffContentVP.AtBottom() {
		scrollDown = diffScrollHintStyle.Render("▼ scroll down (ctrl+d)")
	}

	content := header + scrollUp + stickyLine + "\n" + m.diffContentVP.View()
	if scrollDown != "" {
		content += "\n" + scrollDown
	}

	return borderStyle.
		Width(m.diffRightWidth).
		Height(panelHeight).
		Render(content)
}

func (m *model) updateDiffContent() {
	tree, cursorLine := m.diffFileTreeContent()
	m.diffFileVP.SetContent(tree)

	// Auto-scroll: keep cursor visible in the file list viewport.
	// Only scroll if cursor is within the visible entries range.
	vis := m.visibleDiffEntries()
	if m.diffCursor >= 0 && m.diffCursor < len(vis) {
		vpHeight := m.diffFileVP.Height()
		yOff := m.diffFileVP.YOffset()
		if vpHeight > 0 {
			if cursorLine < yOff {
				m.diffFileVP.SetYOffset(cursorLine)
			} else if cursorLine >= yOff+vpHeight {
				m.diffFileVP.SetYOffset(cursorLine - vpHeight + 1)
			}
		}
	}

	content, funcCtx := m.diffSideBySideContent()
	m.diffContentVP.SetContent(content)
	m.diffFuncCtx = funcCtx
}
