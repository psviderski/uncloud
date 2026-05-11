package tui

import (
	"fmt"
	"os"
)

func PrintWarning(msg string) {
	styledMsg := BoldYellow.Render(fmt.Sprintf("WARNING: %s", msg))
	fmt.Fprintln(os.Stderr, styledMsg)
}
