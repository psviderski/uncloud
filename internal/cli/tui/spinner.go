package tui

import (
	"context"
	"fmt"
	"os"

	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
)

// RunSpinner shows an animated spinner that prints title while running action.
// When the terminal (TTY) is not available, it prints the title as plain text.
// It renders to stderr so stdout stays clean for command output.
func RunSpinner(ctx context.Context, title string, action func(ctx context.Context) error) error {
	// Fall back to plain text when a terminal is not available to avoid the bubbletea /dev/tty error and to keep
	// escape codes out of redirected output.
	if !IsTerminalAvailable() {
		fmt.Fprintln(os.Stderr, title)
		return action(ctx)
	}

	return spinner.New().
		// Leading space offsets the title from the spinner glyph.
		Title(" " + title).
		Type(spinner.MiniDot).
		WithTheme(spinner.ThemeFunc(func(isDark bool) *spinner.Styles {
			return &spinner.Styles{
				Spinner: lipgloss.NewStyle().Foreground(lipgloss.Yellow),
				Title:   lipgloss.NewStyle(),
			}
		})).
		WithOutput(os.Stderr).
		Context(ctx).
		ActionWithErr(action).
		Run()
}
