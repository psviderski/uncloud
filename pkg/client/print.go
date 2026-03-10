package client

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
)

func PrintWarning(msg string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Bright yellow.
	styledMsg := style.Render(fmt.Sprintf("WARNING: %s", msg))
	fmt.Fprintln(os.Stderr, styledMsg)
}
