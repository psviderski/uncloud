package cli

import (
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func Confirm() (bool, error) {
	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(
					"Do you want to continue?",
				).
				Affirmative("Yes!").
				Negative("No").
				Value(&confirmed),
		),
	).WithAccessible(true)
	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}

// IsStdinTerminal checks if the standard input is a terminal (TTY).
func IsStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
