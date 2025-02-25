package machine

import (
	"context"
	"fmt"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"
	"net/netip"
	"uncloud/internal/cli"
	"uncloud/internal/cli/config"
	"uncloud/internal/machine/cluster"
)

type initOptions struct {
	name     string
	network  string
	noCaddy  bool
	publicIP string
	sshKey   string
	cluster  string
}

func NewInitCommand() *cobra.Command {
	opts := initOptions{}
	cmd := &cobra.Command{
		Use:   "init [USER@HOST:PORT]",
		Short: "Initialise a new cluster that consists of the local or remote machine.",
		// TODO: include usage examples of initialising a local and remote machine.
		Args: cobra.MaximumNArgs(1),
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

			return initCluster(cmd.Context(), uncli, remoteMachine, opts)
		},
	}
	cmd.Flags().StringVarP(
		&opts.name, "name", "n", "",
		"Assign a name to the machine.",
	)
	cmd.Flags().StringVar(
		&opts.network, "network", cluster.DefaultNetwork.String(),
		"IPv4 network CIDR to use for machines and services.",
	)
	cmd.Flags().BoolVar(
		&opts.noCaddy, "no-caddy", false,
		"Don't deploy Caddy reverse proxy service to the machine.",
	)
	cmd.Flags().StringVar(
		&opts.publicIP, "public-ip", "auto",
		"Public IP address of the machine for ingress configuration. Use 'auto' for automatic detection, "+
			"blank '' or 'none' to disable ingress on this machine, or specify an IP address.",
	)
	cmd.Flags().StringVarP(
		&opts.sshKey, "ssh-key", "i", "",
		"path to SSH private key for SSH remote login. (default ~/.ssh/id_*)",
	)
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster in the local config if initialising a remote machine.",
	)

	return cmd
}

func initCluster(ctx context.Context, uncli *cli.CLI, remoteMachine *cli.RemoteMachine, opts initOptions) error {
	netPrefix, err := netip.ParsePrefix(opts.network)
	if err != nil {
		return fmt.Errorf("parse network CIDR: %w", err)
	}

	var publicIP *netip.Addr
	switch opts.publicIP {
	case "auto":
		publicIP = &netip.Addr{}
	case "", "none":
		publicIP = nil
	default:
		ip, err := netip.ParseAddr(opts.publicIP)
		if err != nil {
			return fmt.Errorf("parse public IP: %w", err)
		}
		publicIP = &ip
	}

	client, err := uncli.InitCluster(ctx, remoteMachine, opts.cluster, opts.name, netPrefix, publicIP)
	if err != nil {
		return err
	}
	defer client.Close()

	if opts.noCaddy {
		return nil
	}

	// Deploy the Caddy service to the initialised machine.
	// The creation of a deployment plan talks to cluster API. Since the API needs a few moments to become available
	// after cluster initialisation, we keep the user informed during this wait.
	fmt.Println("Waiting for the machine to be ready...")

	d, err := client.NewCaddyDeployment("", nil)
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = d.Run(ctx); err != nil {
			return fmt.Errorf("deploy caddy: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Deploying service %s", d.Spec.Name))
}
