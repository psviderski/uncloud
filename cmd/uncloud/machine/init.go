package machine

import (
	"fmt"
	"github.com/spf13/cobra"
	"net/netip"
	"uncloud/internal/machine"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
)

type initOptions struct {
	name          string
	network       string
	userPublicKey string
	dataDir       string
}

func NewInitCommand() *cobra.Command {
	opts := initOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise a new cluster that consists of the local or remote machine.",
		RunE: func(cmd *cobra.Command, args []string) error {
			netPrefix, err := netip.ParsePrefix(opts.network)
			if err != nil {
				return fmt.Errorf("parse network CIDR: %w", err)
			}

			var users []*pb.User
			if opts.userPublicKey != "" {
				pubKey, uErr := secret.FromHexString(opts.userPublicKey)
				if uErr != nil {
					return fmt.Errorf("parse user's public key: %w", uErr)
				}
				user := &pb.User{
					Network: &pb.NetworkConfig{
						ManagementIp: pb.NewIP(network.ManagementIP(pubKey)),
						PublicKey:    pubKey,
					},
				}
				users = append(users, user)
			}

			// TODO: ideally this should be an RPC call to the machine API via unix socket.
			config := &machine.Config{DataDir: opts.dataDir}
			mach, err := machine.NewMachine(config)
			if err != nil {
				return fmt.Errorf("init machine: %w", err)
			}
			if err = mach.InitCluster(opts.name, netPrefix, users); err != nil {
				return fmt.Errorf("initialise cluster: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine")
	cmd.Flags().StringVar(&opts.network, "network", network.DefaultNetwork.String(),
		"IPv4 network CIDR to use for machines and services")
	cmd.Flags().StringVarP(&opts.userPublicKey, "user-pubkey", "u", "",
		"User's public key which will be able to access the cluster (hex-encoded)")

	cmd.Flags().StringVarP(&opts.dataDir, "data-dir", "d", machine.DefaultDataDir,
		"Directory for storing persistent machine state")
	_ = cmd.MarkFlagDirname("data-dir")

	return cmd
}
