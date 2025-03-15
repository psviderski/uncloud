package cli

import "github.com/charmbracelet/huh"

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
	)
	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}
