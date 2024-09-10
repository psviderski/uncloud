package machine

import (
	"fmt"
	"github.com/spf13/cobra"
	"net/netip"
	"uncloud/internal/cli"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine/network"
)

type initOptions struct {
	name          string
	network       string
	userPublicKey string

	sshKey  string
	cluster string
}

func NewInitCommand() *cobra.Command {
	opts := initOptions{}
	cmd := &cobra.Command{
		Use:  "init [USER@HOST:PORT]",
		Args: cobra.MaximumNArgs(1),
		// TODO: include usage examples of initialising a local and remote machine.
		Short: "Initialise a new cluster that consists of the local or remote machine.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			var remoteMachine *cli.RemoteMachine
			if len(args) > 0 {
				user, host, port, err := config.SSHDestination(args[0]).Parse()
				if err != nil {
					return fmt.Errorf("parse remote machine: %w", err)
				}
				remoteMachine = &cli.RemoteMachine{
					User:    user,
					Host:    host,
					Port:    port,
					KeyPath: opts.sshKey,
				}
			}
			netPrefix, err := netip.ParsePrefix(opts.network)
			if err != nil {
				return fmt.Errorf("parse network CIDR: %w", err)
			}

			return uncli.InitCluster(cmd.Context(), remoteMachine, opts.cluster, opts.name, netPrefix)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine")
	cmd.Flags().StringVar(
		&opts.network, "network", network.DefaultNetwork.String(),
		"IPv4 network CIDR to use for machines and services",
	)
	//cmd.Flags().StringVar(&opts.userPublicKey, "user-pubkey", "",
	//	"User's public key which will be able to access the cluster (hex-encoded)")

	cmd.Flags().StringVarP(
		&opts.sshKey, "ssh-key", "i", "",
		"path to SSH private key for SSH remote login (default ~/.ssh/id_*)",
	)
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster in the local config if initialising a remote machine",
	)

	return cmd
}
