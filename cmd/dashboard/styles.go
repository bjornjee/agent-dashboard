package main

import "github.com/charmbracelet/lipgloss"

// -- Catppuccin Frappé palette --

const (
	catRosewater = lipgloss.Color("#f2d5cf")
	catFlamingo  = lipgloss.Color("#eebebe")
	catPink      = lipgloss.Color("#f4b8e4")
	catMauve     = lipgloss.Color("#ca9ee6")
	catRed       = lipgloss.Color("#e78284")
	catMaroon    = lipgloss.Color("#ea999c")
	catPeach     = lipgloss.Color("#ef9f76")
	catYellow    = lipgloss.Color("#e5c890")
	catGreen     = lipgloss.Color("#a6d189")
	catTeal      = lipgloss.Color("#81c8be")
	catSky       = lipgloss.Color("#99d1db")
	catSapphire  = lipgloss.Color("#85c1dc")
	catBlue      = lipgloss.Color("#8caaee")
	catLavender  = lipgloss.Color("#babbf1")
	catText      = lipgloss.Color("#c6d0f5")
	catSubtext1  = lipgloss.Color("#b5bfe2")
	catSubtext0  = lipgloss.Color("#a5adce")
	catOverlay2  = lipgloss.Color("#949cbb")
	catOverlay1  = lipgloss.Color("#838ba7")
	catOverlay0  = lipgloss.Color("#737994")
	catSurface2  = lipgloss.Color("#626880")
	catSurface1  = lipgloss.Color("#51576d")
	catSurface0  = lipgloss.Color("#414559")
	catBase      = lipgloss.Color("#303446")
	catMantle    = lipgloss.Color("#292c3c")
	catCrust     = lipgloss.Color("#232634")
)

// -- Styles --

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(catOverlay0)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(catSapphire)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(catSurface1).
			Foreground(catText)

	inputColor   = catYellow
	errorColor   = catRed
	runningColor = catBlue
	idleColor    = catOverlay1
	doneColor    = catGreen

	helpStyle      = lipgloss.NewStyle().Foreground(catOverlay1)
	boldStyle      = lipgloss.NewStyle().Bold(true)
	costStyle      = lipgloss.NewStyle().Foreground(catPeach).Bold(true)
	planLabelStyle = lipgloss.NewStyle().Foreground(catMauve).Bold(true)
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
