package machine

import (
	"context"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"net/netip"
	"time"
)

type addOptions struct {
	name     string
	noCaddy  bool
	publicIP string
	sshKey   string
	cluster  string
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

			return add(cmd.Context(), uncli, remoteMachine, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine.")
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
		"Name of the cluster to add the machine to. (default is the current cluster)",
	)
	return cmd
}

func add(ctx context.Context, uncli *cli.CLI, remoteMachine cli.RemoteMachine, opts addOptions) error {
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

	machineClient, err := uncli.AddMachine(ctx, remoteMachine, opts.cluster, opts.name, publicIP)
	if err != nil {
		return err
	}
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

	// Inspect the added machine to get its ID to create a filter for the Caddy deployment.
	minfo, err := machineClient.Inspect(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machine: %w", err)
	}
	filter := func(m *pb.MachineInfo) bool {
		return m.Id == minfo.Id
	}
	// Deploy a Caddy service container to the added machine. If caddy service is already deployed on other machines,
	// use the deployed image version. Otherwise, use the latest version.
	caddyImage := ""
	caddySvc, err := machineClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if !errors.Is(err, client.ErrNotFound) {
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

	d, err := machineClient.NewCaddyDeployment(caddyImage, filter)
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
		_, err := client.ListMachines(ctx)
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
