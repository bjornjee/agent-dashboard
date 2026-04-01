package main

import "github.com/charmbracelet/lipgloss"

// -- Theme palette --
//
// Generic color names mapped to a specific theme. To switch themes,
// update the hex values below — no other files need to change.
//
// Current theme: Catppuccin Frappé
// Reference: https://catppuccin.com/palette

const (
	themeRosewater = lipgloss.Color("#f2d5cf")
	themeFlamingo  = lipgloss.Color("#eebebe")
	themePink      = lipgloss.Color("#f4b8e4")
	themeMauve     = lipgloss.Color("#ca9ee6")
	themeRed       = lipgloss.Color("#e78284")
	themeMaroon    = lipgloss.Color("#ea999c")
	themePeach     = lipgloss.Color("#ef9f76")
	themeYellow    = lipgloss.Color("#e5c890")
	themeGreen     = lipgloss.Color("#a6d189")
	themeTeal      = lipgloss.Color("#81c8be")
	themeSky       = lipgloss.Color("#99d1db")
	themeSapphire  = lipgloss.Color("#85c1dc")
	themeBlue      = lipgloss.Color("#8caaee")
	themeLavender  = lipgloss.Color("#babbf1")
	themeText      = lipgloss.Color("#c6d0f5")
	themeSubtext1  = lipgloss.Color("#b5bfe2")
	themeSubtext0  = lipgloss.Color("#a5adce")
	themeOverlay2  = lipgloss.Color("#949cbb")
	themeOverlay1  = lipgloss.Color("#838ba7")
	themeOverlay0  = lipgloss.Color("#737994")
	themeSurface2  = lipgloss.Color("#626880")
	themeSurface1  = lipgloss.Color("#51576d")
	themeSurface0  = lipgloss.Color("#414559")
	themeBase      = lipgloss.Color("#303446")
	themeMantle    = lipgloss.Color("#292c3c")
	themeCrust     = lipgloss.Color("#232634")
)

// -- Styles --

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(themeOverlay0)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(themeSapphire)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(themeSurface1).
			Foreground(themeText)

	inputColor   = themeYellow
	errorColor   = themeRed
	runningColor = themeBlue
	idleColor    = themeOverlay1
	doneColor    = themeGreen

	helpStyle      = lipgloss.NewStyle().Foreground(themeOverlay1)
	boldStyle      = lipgloss.NewStyle().Bold(true)
	costStyle      = lipgloss.NewStyle().Foreground(themePeach).Bold(true)
	planLabelStyle = lipgloss.NewStyle().Foreground(themeMauve).Bold(true)

	diffCollapsedStyle  = lipgloss.NewStyle().Foreground(themeOverlay1).Italic(true)
	diffScrollHintStyle = lipgloss.NewStyle().Foreground(themeSapphire)
)

type stateIcon struct {
	icon  string
	color lipgloss.Color
}

var stateIcons = map[string]stateIcon{
	"input":   {"!", inputColor},
	"error":   {"✗", errorColor},
	"running": {"▶", runningColor},
	"idle":    {"○", idleColor},
	"done":    {"✓", doneColor},
}

var groupHeaders = map[int]struct {
	label string
	color lipgloss.Color
}{
	1: {"NEEDS ATTENTION", inputColor},
	2: {"RUNNING", runningColor},
	3: {"COMPLETED", doneColor},
}
