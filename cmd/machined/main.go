package main

import (
	"github.com/spf13/cobra"
	"uncloud/cmd/machined/install"
	"uncloud/internal/machine/daemon"
)

func main() {
	var dataDir string
	cmd := &cobra.Command{
		Use:           "machined",
		Short:         "Uncloud machine daemon.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Run(dataDir)
		},
	}
	cmd.PersistentFlags().StringVarP(&dataDir, "data-dir", "d", daemon.DefaultDataDir, "Directory to store machine state")
	_ = cmd.MarkFlagDirname("data-dir")
	cmd.AddCommand(
		install.NewCommand(&dataDir),
	)
	cobra.CheckErr(cmd.Execute())
}
