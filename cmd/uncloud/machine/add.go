package machine

import (
	"context"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type addOptions struct {
	name    string
	user    string
	port    int
	sshKey  string
	cluster string
}

func NewAddCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		// TODO: add support for [USER@]HOST[:PORT] syntax
		Use:   "add HOST",
		Short: "Add a new machine to a cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return add(cmd.Context(), uncli, args[0], opts)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine")
	cmd.Flags().StringVarP(&opts.user, "user", "u", "root", "Username for SSH remote login")
	cmd.Flags().IntVarP(&opts.port, "port", "p", 22, "Port for SSH remote login")
	cmd.Flags().StringVarP(&opts.sshKey, "ssh-key", "i", "",
		"path to SSH private key for SSH remote login (default ~/.ssh/id_*)")
	cmd.Flags().StringVarP(&opts.cluster, "cluster", "c", "",
		"Name of the cluster to add the machine to (default is the current cluster)")
	return cmd
}

func add(ctx context.Context, uncli *cli.CLI, host string, opts addOptions) error {
	return uncli.AddMachine(ctx, opts.cluster, opts.name, opts.user, host, opts.port, opts.sshKey)
}
