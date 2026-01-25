package client

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uncloud/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
)

type AddOptions struct {
	Destination string
	Name        string
	PublicIP    string
	SSHKey      string
	Version     string
}

const (
	// PublicIPNone is the value used to indicate removal of public IP
	PublicIPNone = "none"
)

func AddMachine(ctx context.Context, opts AddOptions) error {
	uncli, err := cli.New("", nil, "")
	if err != nil {
		return fmt.Errorf("create cli: %w", err)
	}

	user, host, port, err := config.SSHDestination(opts.Destination).Parse()
	if err != nil {
		return fmt.Errorf("parse remote machine: %w", err)
	}
	remoteMachine := &cli.RemoteMachine{
		User:      user,
		Host:      host,
		Port:      port,
		KeyPath:   opts.SSHKey,
		UseSSHCLI: false,
	}

	return add(ctx, uncli, remoteMachine, opts)
}

func add(ctx context.Context, uncli *cli.CLI, remoteMachine *cli.RemoteMachine, opts AddOptions) error {
	var publicIP *netip.Addr
	switch opts.PublicIP {
	case "auto":
		publicIP = &netip.Addr{}
	case "", PublicIPNone:
		publicIP = nil
	default:
		ip, err := netip.ParseAddr(opts.PublicIP)
		if err != nil {
			return fmt.Errorf("parse public IP: %w", err)
		}
		publicIP = &ip
	}

	clusterClient, machineClient, err := uncli.AddMachine(ctx, cli.AddMachineOptions{
		MachineName:   opts.Name,
		PublicIP:      publicIP,
		RemoteMachine: remoteMachine,
		SkipInstall:   false,
		Version:       opts.Version,
		AutoConfirm:   true,
	})
	if err != nil {
		return err
	}
	defer clusterClient.Close()
	defer machineClient.Close()

	// Wait for the cluster to be initialised on the machine to be able to deploy the Caddy service.
	err = machineClient.WaitClusterReady(ctx, 5*time.Minute)
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

	d, err := clusterClient.NewCaddyDeployment(caddyImage, "", api.Placement{})
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	fmt.Println()
	if len(plan.Operations) == 0 {
		fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
	} else {
		// Initialise a machine and container name resolver to properly format the plan output.
		resolver, err := clusterClient.ServiceOperationNameResolver(ctx, caddySvc)
		if err != nil {
			return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
		}

		fmt.Println("caddy deployment plan:")
		fmt.Println(plan.Format(resolver))
		fmt.Println()

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
