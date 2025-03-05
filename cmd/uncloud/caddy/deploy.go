package caddy

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/huh"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"
	"maps"
	"slices"
	"strings"
	"uncloud/internal/cli"
	"uncloud/internal/cli/client"
	"uncloud/internal/machine/api/pb"
)

type deployOptions struct {
	image   string
	machine string
	cluster string
}

func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.",
		Long: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.\n" +
			"A rolling update is performed when updating existing containers to minimise disruption.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return deploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "",
		"Caddy Docker image to deploy. (default caddy:LATEST_VERSION)")
	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Machine names to deploy to (comma-separated). (default is all machines)")
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to deploy to. (default is the current cluster)",
	)

	return cmd
}

func deploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	svc, err := clusterClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("inspect caddy service: %w", err)
		}
		fmt.Printf("Service: %s (not running)\n", client.CaddyServiceName)
	} else {
		fmt.Printf("Service: %s (%s mode)\n", svc.Name, svc.Mode)

		// Collect unique images of all containers in the running caddy service.
		images := make(map[string]struct{}, len(svc.Containers))
		for _, c := range svc.Containers {
			images[c.Container.Config.Image] = struct{}{}
		}
		currentImages := slices.Collect(maps.Keys(images))

		if len(currentImages) > 1 {
			commaSeparatedImages := strings.Join(currentImages, ", ")
			fmt.Printf("Current images (multiple versions detected): %s\n", commaSeparatedImages)
		} else {
			fmt.Printf("Current image: %s\n", currentImages[0])
		}
	}

	if opts.image != "" {
		fmt.Printf("Target image: %s\n", opts.image)
	}

	fmt.Println()
	fmt.Println("Preparing a deployment plan...")

	var filter client.MachineFilter
	if opts.machine != "" {
		machines := strings.Split(opts.machine, ",")
		for i, m := range machines {
			machines[i] = strings.TrimSpace(m)
		}
		filter = machineFilter(machines)
	}

	d, err := clusterClient.NewCaddyDeployment(opts.image, filter)
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	if opts.image == "" {
		fmt.Printf("Target image: %s (latest stable)\n", d.Spec.Container.Image)
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		if errors.Is(err, client.ErrNoMatchingMachines) {
			return fmt.Errorf("no machines found matching: %s", opts.machine)
		}
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		if opts.machine != "" {
			fmt.Printf("%s service is up to date on selected machines.\n", client.CaddyServiceName)
		} else {
			fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
		}
	} else {
		if svc.ID == "" {
			if opts.machine != "" {
				fmt.Println("This will run a Caddy container on selected machines.")
			} else {
				fmt.Println("This will run a Caddy container on each machine.")
			}
		} else {
			if opts.machine != "" {
				fmt.Println("This will perform a rolling update of Caddy containers on selected machines.")
			} else {
				fmt.Println("This will perform a rolling update of Caddy containers on each machine.")
			}
		}

		// Initialise a machine and container name resolver to properly format the plan output.
		resolver, err := clusterClient.ServiceOperationNameResolver(ctx, svc)
		if err != nil {
			return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
		}

		fmt.Println()
		fmt.Println("Deployment plan:")
		fmt.Println(plan.Format(resolver))
		fmt.Println()

		confirmed, err := confirm()
		if err != nil {
			return fmt.Errorf("confirm deployment: %w", err)
		}
		if !confirmed {
			fmt.Println("Cancelled. No changes were made.")
			return nil
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
	return UpdateDomainRecords(ctx, clusterClient, uncli.ProgressOut())
}

func UpdateDomainRecords(ctx context.Context, clusterClient *client.Client, progressOut *streams.Out) error {
	if _, err := clusterClient.GetDomain(ctx); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			fmt.Println("Skipping DNS records update as no cluster domain is reserved (see 'uc dns').")
			return nil
		}
		return fmt.Errorf("get cluster domain: %w", err)
	}

	fmt.Println("Updating cluster domain records in Uncloud DNS to point to machines running caddy service...")
	// TODO: split the method into two: one to get the records and one to update them to ask for update confirmation.

	var records []*pb.DNSRecord
	err := progress.RunWithTitle(ctx, func(ctx context.Context) error {
		var err error
		records, err = clusterClient.CreateIngressRecords(ctx, client.CaddyServiceName)
		return err
	}, progressOut, "Verifying internet access to caddy service")
	if err != nil {
		if errors.Is(err, client.ErrNoReachableMachines) {
			fmt.Println()
			fmt.Println("DNS records could not be updated as there are no internet-reachable machines running " +
				"caddy containers.")
			fmt.Println()
			fmt.Println("Possible solutions:")
			fmt.Println("- Ensure your machines have public IP addresses")
			fmt.Println("- Use --public-ip flag when adding machines to override the automatically detected IPs")
			fmt.Println("- Check firewall settings on your machines")
			fmt.Println("- Configure port forwarding if behind NAT")
			fmt.Println("- Retry Caddy deployment after resolving connectivity issues with 'uc caddy deploy'")
			fmt.Println()
			fmt.Println("Your services will not be accessible from the internet until at least one machine " +
				"becomes reachable.")
		}
		return fmt.Errorf("failed to update DNS records pointing to caddy service: %w", err)
	}

	fmt.Println()
	fmt.Println("DNS records updated to use only the internet-reachable machines running caddy service:")
	for _, r := range records {
		fmt.Printf("  %s  %s → %s\n", r.Name, r.Type, strings.Join(r.Values, ", "))
	}

	return nil
}

func confirm() (bool, error) {
	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(
					"Do you want to continue?",
				).
				Affirmative("Yes!").
				Negative("No").
				Value(&confirmed),
		),
	)
	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}

func machineFilter(machines []string) client.MachineFilter {
	if len(machines) == 0 {
		return nil
	}
	return func(m *pb.MachineInfo) bool {
		return slices.Contains(machines, m.Name)
	}
}
