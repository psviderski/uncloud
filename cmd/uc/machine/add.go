package machine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uc/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type addOptions struct {
	name        string
	noCaddy     bool
	noInstall   bool
	publicIP    string
	sshKey      string
	version     string
	wgEndpoints []string
	wgPort      int
	wgMTU       int
	yes         bool
}

func NewAddCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		Use:   "add [USER@]HOST[:PORT]",
		Short: "Add a remote machine to a cluster.",
		Long: `Add a new machine to an existing Uncloud cluster.

By default, it installs Docker and the Uncloud daemon on the machine over SSH unless --no-install
is specified, which assumes they are already installed and running.

Connection methods:
  [ssh://]user@host   - Use system 'ssh' command with full SSH config support (default, no prefix required)
  ssh+go://user@host  - Use Go's built-in SSH library`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.BindEnvToFlag(cmd, "yes", "UNCLOUD_AUTO_CONFIRM")

			uncli := cmd.Context().Value("cli").(*cli.CLI)

			// Determine connection mode and strip scheme.
			destination := args[0]
			useSSHGo := strings.HasPrefix(destination, "ssh+go://")
			destination = strings.TrimPrefix(destination, "ssh+go://")
			destination = strings.TrimPrefix(destination, "ssh+cli://")
			destination = strings.TrimPrefix(destination, "ssh://")

			user, host, port, err := config.SSHDestination(destination).Parse()
			if err != nil {
				return fmt.Errorf("parse remote machine: %w", err)
			}
			remoteMachine := &cli.RemoteMachine{
				User:     user,
				Host:     host,
				Port:     port,
				KeyPath:  opts.sshKey,
				UseSSHGo: useSSHGo,
			}

			return add(cmd.Context(), uncli, remoteMachine, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the machine. (default is the machine's hostname)")
	cmd.Flags().BoolVar(
		&opts.noCaddy, "no-caddy", false,
		"Don't deploy Caddy reverse proxy service to the machine.",
	)
	cmd.Flags().BoolVar(
		&opts.noInstall, "no-install", false,
		"Skip installation of Docker and the Uncloud daemon on the machine. "+
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
	cmd.Flags().StringSliceVar(
		&opts.wgEndpoints, "wg-endpoint", nil,
		"WireGuard endpoint address that other machines in the cluster should use to establish "+
			"WireGuard connections\n"+
			"to this machine. This doesn't change the address/port WireGuard listens on the machine.\n"+
			"Format: IP, IP:PORT, IPv6, or [IPv6]:PORT. Default port is the value of --wg-port if omitted.\n"+
			"Multiple endpoints can be specified by repeating the flag or using a comma-separated list.\n"+
			"Defaults to the auto-detected public and routable machine IPs.",
	)
	cmd.Flags().IntVar(
		&opts.wgMTU, "wg-mtu", 0,
		"MTU of the WireGuard network interface on the machine. (default auto-detects the optimal value)",
	)
	cmd.Flags().IntVar(
		&opts.wgPort, "wg-port", network.DefaultWireGuardPort,
		"UDP port WireGuard listens on for incoming connections from other machines.",
	)
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Auto-confirm prompts (e.g., resetting an already initialised machine).\n"+
			"Should be explicitly set when running non-interactively, e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]")

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

	if opts.wgPort < 1 || opts.wgPort > 65535 {
		return fmt.Errorf("invalid WireGuard port %d: must be between 1 and 65535", opts.wgPort)
	}
	if opts.wgMTU != 0 && (opts.wgMTU < network.MinWireGuardMTU || opts.wgMTU > 65535) {
		return fmt.Errorf("invalid WireGuard MTU %d: must be 0 (auto-detect) or between %d and 65535",
			opts.wgMTU, network.MinWireGuardMTU)
	}
	addOpts := cli.AddMachineOptions{
		MachineName:   opts.name,
		PublicIP:      publicIP,
		RemoteMachine: remoteMachine,
		SkipInstall:   opts.noInstall,
		Version:       opts.version,
		WireguardMTU:  opts.wgMTU,
		WireguardPort: opts.wgPort,
		AutoConfirm:   opts.yes,
	}
	if len(opts.wgEndpoints) > 0 {
		expanded := cli.ExpandCommaSeparatedValues(opts.wgEndpoints)
		endpoints, err := cli.ParseWireGuardEndpoints(expanded, uint16(opts.wgPort))
		if err != nil {
			return fmt.Errorf("parse WireGuard endpoint (--wg-endpoint): %w", err)
		}
		addOpts.WireguardEndpoints = endpoints
	}

	clusterClient, machineClient, err := uncli.AddMachine(ctx, addOpts)
	if err != nil {
		return err
	}
	defer clusterClient.Close()
	defer machineClient.Close()

	if opts.noCaddy {
		return nil
	}

	// Wait for the cluster to be initialised on the machine to be able to deploy the Caddy service.
	err = tui.RunSpinner(ctx, "Waiting for the machine to join the cluster...", func(ctx context.Context) error {
		return machineClient.WaitClusterReady(ctx, 5*time.Minute)
	})
	if err != nil {
		return fmt.Errorf("wait for machine to join the cluster: %w", err)
	}
	fmt.Println("Machine joined the cluster.")

	// TODO: scale the existing Caddy service to the new machine instead of running a new deployment
	//  that may cause a small downtime.
	// Deploy a Caddy service container to the added machine. If caddy service is already deployed on other machines,
	// use the deployed image version.
	// NOTE: We use the cluster client to inspect and scale the Caddy service because the newly added machine may have
	// issues accessing the Machine API of existing machines in the cluster.
	// See the issue for more details: https://github.com/psviderski/uncloud/issues/65.
	caddyImage := ""
	caddySvc, err := clusterClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			// Caddy service is not deployed.
			return nil
		}
		return fmt.Errorf("inspect caddy service: %w", err)
	}
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

	fmt.Println()
	fmt.Println("Preparing Caddy deployment...")
	d, err := clusterClient.NewCaddyDeployment(caddyImage, "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
	} else {
		fmt.Println(tui.Bold.Underline(true).Render("Deployment plan"))
		fmt.Println()
		fmt.Print(plan.Format())

		summary := plan.FormatSummary()
		fmt.Println(tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))))
		fmt.Println(summary)
		fmt.Println()

		if !opts.yes {
			confirmed, err := tui.Confirm("Proceed with deployment?")
			if err != nil {
				return fmt.Errorf("confirm deployment: %w", err)
			}
			if !confirmed {
				return cli.Cancelled("Caddy deploy cancelled. The machine has been added to the cluster.")
			}
		}

		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if _, err = d.Run(ctx); err != nil {
				return fmt.Errorf("deploy caddy: %w", err)
			}
			return nil
		}, uncli.ProgressOut(), fmt.Sprintf("Deploying service %s (%s mode)", d.Spec.Name, d.Spec.Mode))
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return caddy.UpdateDomainRecords(ctx, machineClient, uncli.ProgressOut())
}
