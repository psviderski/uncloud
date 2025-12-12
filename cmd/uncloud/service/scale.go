package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type scaleOptions struct {
	service  string
	replicas uint
}

func NewScaleCommand(groupID string) *cobra.Command {
	opts := scaleOptions{}
	cmd := &cobra.Command{
		Use:   "scale SERVICE REPLICAS",
		Short: "Scale a replicated service by changing the number of replicas.",
		Long:  "Scale a replicated service by changing the number of replicas. Scaling down requires confirmation.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.service = args[0]
			replicas, err := strconv.ParseUint(args[1], 10, 0)
			if err != nil {
				return fmt.Errorf("invalid number of replicas: %w", err)
			}
			opts.replicas = uint(replicas)

			return scale(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}

	return cmd
}

func scale(ctx context.Context, uncli *cli.CLI, opts scaleOptions) error {
	if opts.replicas == 0 {
		return fmt.Errorf(
			"scaling to zero replicas is not supported. This would effectively remove the service without preserving "+
				"its configuration, making it impossible to scale back up. Uncloud derives the service configuration "+
				"from existing containers. Use 'uc rm %s' instead if you want to remove the service",
			opts.service,
		)
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	svc, err := clusterClient.InspectService(ctx, opts.service)
	if err != nil {
		return fmt.Errorf("inspect service '%s': %w", opts.service, err)
	}

	if svc.Mode != api.ServiceModeReplicated {
		return fmt.Errorf("scaling is only supported for services in %s mode, service '%s' is in %s mode",
			api.ServiceModeReplicated, svc.Name, svc.Mode)
	}

	currentReplicas := uint(len(svc.Containers))

	if currentReplicas == opts.replicas {
		fmt.Printf("Service '%s' already has %d replicas. No changes required.\n", svc.Name, currentReplicas)
		return nil
	}

	// TODO: Check if all containers have the same spec. If not, prompt user to choose which one to scale.
	//  This can happen if a service deployment failed midway and some containers were not updated.
	spec := svc.Containers[0].Container.ServiceSpec
	spec.Replicas = opts.replicas
	deployment := clusterClient.NewDeployment(spec, nil)
	plan, err := deployment.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Printf("Service '%s' is already scaled to %d replicas.\n", svc.Name, opts.replicas)
		return nil
	}

	if opts.replicas < currentReplicas {
		// Initialise a machine and container name resolver to properly format the plan output.
		resolver, err := clusterClient.ServiceOperationNameResolver(ctx, svc)
		if err != nil {
			return fmt.Errorf("create machine and container name resolver for service operations: %w", err)
		}

		fmt.Printf("Scaling plan for service %s (%d → %d replicas):\n", svc.Name, currentReplicas, opts.replicas)
		fmt.Println(plan.Format(resolver))
		fmt.Println()

		// Ask for confirmation before scaling down as it may cause data loss.
		confirmed, err := confirm()
		if err != nil {
			return fmt.Errorf("confirm scaling: %w", err)
		}
		if !confirmed {
			fmt.Println("Cancelled. No changes were made.")
			return nil
		}
	}

	title := fmt.Sprintf("Scaling service %s (%d → %d replicas)", svc.Name, currentReplicas, opts.replicas)
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = deployment.Run(ctx); err != nil {
			return fmt.Errorf("deploy service: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), title)
	if err != nil {
		return err
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
	).WithAccessible(true)
	if err := form.Run(); err != nil {
		return false, err
	}

	return confirmed, nil
}
