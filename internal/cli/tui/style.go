package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/distribution/reference"
)

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

// FormatImage renders an image reference with the given style, using a faint colon separator for tagged images.
// Returns the styled original string if parsing fails.
func FormatImage(image string, style lipgloss.Style) string {
	ref, err := reference.ParseDockerRef(image)
	if err != nil {
		return style.Render(image)
	}

	if tagged, ok := ref.(reference.NamedTagged); ok {
		return style.Render(reference.FamiliarName(ref)) +
			Faint.Render(":") +
			style.Render(tagged.Tag())
	}
	return style.Render(reference.FamiliarString(ref))
}
