package tui

import "charm.land/lipgloss/v2"

var (
	Faint  = lipgloss.NewStyle().Faint(true)
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Yellow)

	Bold       = lipgloss.NewStyle().Bold(true)
	BoldRed    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red)
	BoldGreen  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Green)
	BoldYellow = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Yellow)

	NameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("152"))
)
