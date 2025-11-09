package machine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type addOptions struct {
	name      string
	noCaddy   bool
	noInstall bool
	publicIP  string
	sshKey    string
	version   string
}

func NewAddCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		Use:   "add [USER@]HOST[:PORT]",
		Short: "Add a remote machine to a cluster.",
		Long: `Add a new machine to an existing Uncloud cluster.

Connection methods:
  ssh://user@host       - Use built-in SSH library (default, no prefix required)
  ssh+cli://user@host   - Use system SSH command (supports ProxyJump, SSH config)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			// Determine if SSH CLI needs to be used and strip scheme
			destination := args[0]
			useSSHCLI := strings.HasPrefix(destination, "ssh+cli://")
			destination = strings.TrimPrefix(destination, "ssh+cli://")
			destination = strings.TrimPrefix(destination, "ssh://")

			user, host, port, err := config.SSHDestination(destination).Parse()
			if err != nil {
				return fmt.Errorf("parse remote machine: %w", err)
			}
			remoteMachine := &cli.RemoteMachine{
				User:      user,
				Host:      host,
				Port:      port,
				KeyPath:   opts.sshKey,
				UseSSHCLI: useSSHCLI,
			}

			return add(cmd.Context(), uncli, remoteMachine, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine.")
	cmd.Flags().BoolVar(
		&opts.noCaddy, "no-caddy", false,
		"Don't deploy Caddy reverse proxy service to the machine.",
	)
	cmd.Flags().BoolVar(
		&opts.noInstall, "no-install", false,
		"Skip installation of Docker, Uncloud daemon, and dependencies on the machine. "+
			"Assumes they're already installed and running.",
	)
	cmd.Flags().StringVar(
		&opts.publicIP, "public-ip", "auto",
		"Public IP address of the machine for ingress configuration. Use 'auto' for automatic detection, "+
			fmt.Sprintf("blank '' or '%s' to disable ingress on this machine, or specify an IP address.", PublicIPNone),
	)
	cmd.Flags().StringVarP(
		&opts.sshKey, "ssh-key", "i", "",
		fmt.Sprintf("Path to SSH private key for remote login (if not already added to SSH agent). (default %q)",
			cli.DefaultSSHKeyPath),
	)
	cmd.Flags().StringVar(
		&opts.version, "version", "latest",
		"Version of the Uncloud daemon to install on the machine.",
	)

	return cmd
}

func add(ctx context.Context, uncli *cli.CLI, remoteMachine *cli.RemoteMachine, opts addOptions) error {
	var publicIP *netip.Addr
	switch opts.publicIP {
	case "auto":
		publicIP = &netip.Addr{}
	case "", PublicIPNone:
		publicIP = nil
	default:
		ip, err := netip.ParseAddr(opts.publicIP)
		if err != nil {
			return fmt.Errorf("parse public IP: %w", err)
		}
		publicIP = &ip
	}

	clusterClient, machineClient, err := uncli.AddMachine(ctx, cli.AddMachineOptions{
		MachineName:   opts.name,
		PublicIP:      publicIP,
		RemoteMachine: remoteMachine,
		SkipInstall:   opts.noInstall,
		Version:       opts.version,
	})
	if err != nil {
		return err
	}
	defer clusterClient.Close()
	defer machineClient.Close()

	if opts.noCaddy {
		return nil
	}

	// Wait for the cluster to be initialised to be able to deploy the Caddy service.
	fmt.Println("Waiting for the machine to be ready...")
	fmt.Println()
	if err = waitClusterInitialised(ctx, machineClient); err != nil {
		return fmt.Errorf("wait for cluster to be initialised on machine: %w", err)
	}

	// Deploy a Caddy service container to the added machine. If caddy service is already deployed on other machines,
	// use the deployed image version. Otherwise, use the latest version.
	// NOTE: We use the cluster client to inspect and scale the Caddy service because the newly added machine may have
	// issues accessing the Machine API of existing machines in the cluster.
	// See the issue for more details: https://github.com/psviderski/uncloud/issues/65.
	caddyImage := ""
	caddySvc, err := clusterClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("inspect caddy service: %w", err)
		}
	} else {
		caddyImage = caddySvc.Containers[0].Container.Config.Image
		// Find the latest created container and use its image.
		var latestCreated time.Time
		for _, c := range caddySvc.Containers[1:] {
			created, err := time.Parse(time.RFC3339Nano, c.Container.Created)
			if err != nil {
				continue
			}
			if created.After(latestCreated) {
				latestCreated = created
				caddyImage = c.Container.Config.Image
			}
		}
	}

	// TODO: scale the existing Caddy service to the new machine instead of running a new deployment
	//  that may cause a small downtime.
	d, err := clusterClient.NewCaddyDeployment(caddyImage, "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = d.Run(ctx); err != nil {
			return fmt.Errorf("deploy caddy: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Deploying service %s", d.Spec.Name))
	if err != nil {
		return err
	}

	fmt.Println()
	return caddy.UpdateDomainRecords(ctx, machineClient, uncli.ProgressOut())
}

func waitClusterInitialised(ctx context.Context, client *client.Client) error {
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(5*time.Minute),
	), ctx)

	check := func() error {
		_, err := client.ListMachines(ctx, nil)
		if err == nil {
			return nil
		}

		statusErr := status.Convert(err)
		if statusErr.Code() == codes.FailedPrecondition {
			return err
		}
		return backoff.Permanent(err)
	}

	return backoff.Retry(check, boff)
}
