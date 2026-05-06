package output

import (
	"fmt"

	"github.com/spf13/cobra"
)

func Flag(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVarP(value, "output", "o", "",
		"Output format. Currently only 'json' is supported.")
}

func FlagValue(value string) error {
	switch value {
	case "json":
		return nil
	case "":
		return nil
	}
	return fmt.Errorf("invalid --output format, want 'json'")
}
