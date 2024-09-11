package machine

import (
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
	"uncloud/internal/cli/config"
)

type addOptions struct {
	name    string
	sshKey  string
	cluster string
}

func NewAddCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		Use:   "add [USER@]HOST[:PORT]",
		Short: "Add a remote machine to a cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			user, host, port, err := config.SSHDestination(args[0]).Parse()
			if err != nil {
				return fmt.Errorf("parse remote machine: %w", err)
			}
			remoteMachine := cli.RemoteMachine{
				User:    user,
				Host:    host,
				Port:    port,
				KeyPath: opts.sshKey,
			}

			return uncli.AddMachine(cmd.Context(), remoteMachine, opts.cluster, opts.name)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine")
	cmd.Flags().StringVarP(
		&opts.sshKey, "ssh-key", "i", "",
		"path to SSH private key for SSH remote login (default ~/.ssh/id_*)",
	)
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to add the machine to (default is the current cluster)",
	)
	return cmd
}
