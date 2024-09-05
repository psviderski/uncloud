package machine

import (
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/machine"
	"uncloud/internal/machine/daemon"
)

type tokenOptions struct {
	dataDir string
}

func NewTokenCommand() *cobra.Command {
	opts := tokenOptions{}
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the local machine's token for adding it to a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := daemon.MachineToken(opts.dataDir)
			if err != nil {
				return fmt.Errorf("get machine token: %w", err)
			}
			tokenStr, err := token.String()
			if err != nil {
				return fmt.Errorf("encode machine token: %w", err)
			}
			fmt.Println(tokenStr)
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.dataDir, "data-dir", "d", machine.DefaultDataDir,
		"Directory for storing persistent machine state")
	_ = cmd.MarkFlagDirname("data-dir")

	return cmd
}
