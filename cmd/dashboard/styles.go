package main

import "github.com/charmbracelet/lipgloss"

// -- Styles --

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))

	inputColor   = lipgloss.Color("220")
	errorColor   = lipgloss.Color("196")
	runningColor = lipgloss.Color("75")
	idleColor    = lipgloss.Color("242")
	doneColor    = lipgloss.Color("82")

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	boldStyle = lipgloss.NewStyle().Bold(true)
	costStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
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
