package install

import (
	"github.com/spf13/cobra"
	"uncloud/internal/machine"
	"uncloud/internal/machine/daemon"
)

type Options struct {
	uncloudID     string
	uncloudSecret string
	network       string
}

func NewCommand(dataDir *string) *cobra.Command {
	opts := Options{}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install OS dependencies and configure Uncloud machine.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return install(*dataDir, opts)
		},
	}
	cmd.Flags().StringVar(&opts.uncloudID, "id", "", "Globally unique identifier for the uncloud this machine belongs to")
	_ = cmd.MarkFlagRequired("id")
	// TODO: read secret from file for security reasons (bash history).
	cmd.Flags().StringVar(&opts.uncloudSecret, "secret", "", "Shared secret for the uncloud this machine belongs to")
	_ = cmd.MarkFlagRequired("secret")
	cmd.Flags().StringVar(&opts.network, "network", "", "IPv4 network in CIDR format to use for the machine network")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

func install(dataDir string, opts Options) error {
	cfg := daemon.Config{
		UncloudID:     opts.uncloudID,
		UncloudSecret: opts.uncloudSecret,
		Network:       opts.network,
	}
	return machine.Install(dataDir, cfg)
}
