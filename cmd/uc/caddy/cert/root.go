package cert

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Inspect certificates in cluster storage for Caddy.",
	}
	cmd.AddCommand(NewListCommand())
	return cmd
}
