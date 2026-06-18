package tui

import (
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// Confirm shows a confirmation prompt with a yellow-styled title.
// If title is empty, it defaults to "Do you want to continue?".
func Confirm(title string) (bool, error) {
	if title == "" {
		title = "Do you want to continue?"
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes!").
				Negative("No").
				Value(&confirmed),
		),
	).WithTheme(ThemeConfirm()).
		WithAccessible(true).
		// Render to stderr so stdout stays clean for command output.
		WithOutput(os.Stderr)
	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}

// ThemeConfirm returns a huh theme with a bold yellow title style for the confirmation prompt.
func ThemeConfirm() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)
		t.Focused.Title = t.Focused.Title.Foreground(lipgloss.Yellow).Bold(true)
		return t
	})
}

// ThemeConfirmDanger returns a huh theme with a bold red title style for dangerous confirmation prompts.
func ThemeConfirmDanger() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)
		t.Focused.Title = t.Focused.Title.Foreground(lipgloss.Red).Bold(true)
		return t
	})
}

// IsTerminalAvailable reports whether the control terminal (TTY) is available so an interactive TUI can run.
func IsTerminalAvailable() bool {
	// Bubbletea interactive programs read keyboard input from stdin and render to stdout (default) or stderr (uncloud),
	// so both must be terminals. In particular, bubbletea falls back to opening /dev/tty when stdin is not a terminal,
	// which fails when there is no controlling terminal. See https://github.com/psviderski/uncloud/issues/386
	return IsStdinTerminal() && IsStderrTerminal()
}

// IsStdinTerminal checks if the standard input is a terminal (TTY).
func IsStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// IsStdoutTerminal checks if the standard output is a terminal (TTY).
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsStderrTerminal checks if the standard error is a terminal (TTY).
func IsStderrTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// TerminalWidth returns the width of the terminal.
// Returns 0 if stdout is not a terminal or the width cannot be determined.
func TerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}
