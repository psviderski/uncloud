package tui

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
)

func PrintWarning(msg string) {
	styledMsg := BoldYellow.Render(fmt.Sprintf("WARNING: %s", msg))
	lipgloss.Fprintln(os.Stderr, styledMsg)
}
