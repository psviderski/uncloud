package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	dockeropts "github.com/docker/cli/opts"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	service string

	cpu                dockeropts.NanoCPUs
	cpuChanged         bool
	memory             dockeropts.MemBytes
	memoryChanged      bool
	memoryReservation  dockeropts.MemBytes
	reservationChanged bool
	labelAdd           []string
	labelRm            []string
}

func NewUpdateCommand() *cobra.Command {
	opts := updateOptions{}
	cmd := &cobra.Command{
		Use:   "update SERVICE",
		Short: "Update a service in place without recreating its containers.",
		Long: `Update service configuration in place without recreating its containers.

A CPU or memory limit can be changed but not removed. To remove a limit or change anything else,
redeploy the service with 'uc deploy'`,
		Example: `  # Change the CPU and memory limits.
  uc service update web --cpu 0.5 --memory 512m

  # Add or change a service label and remove another one.
  uc service update web --label-add tier=frontend --label-rm legacy`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.service = args[0]
			opts.cpuChanged = cmd.Flag("cpu").Changed
			opts.memoryChanged = cmd.Flag("memory").Changed
			opts.reservationChanged = cmd.Flag("memory-reservation").Changed

			return update(cmd.Context(), uncli, opts)
		},
		ValidArgsFunction: func(
			cmd *cobra.Command, args []string, toComplete string,
		) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Services(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.Flags().Var(&opts.cpu, "cpu",
		"Maximum number of CPU cores a service container can use.\n"+
			"Fractional values are allowed: 0.5 for half a core or 2.25 for two and a quarter cores.")
	cmd.Flags().Var(&opts.memory, "memory",
		"Maximum amount of memory a service container can use. Value is a positive integer with\n"+
			"optional unit suffix (b, k, m, g). Default unit is bytes if no suffix specified.\n"+
			"Examples: 1073741824, 1024m, 1g (all equal 1 gibibyte)")
	cmd.Flags().Var(&opts.memoryReservation, "memory-reservation",
		"Minimum amount of memory reserved for a service container. Value is a positive integer with\n"+
			"optional unit suffix (b, k, m, g). Default unit is bytes if no suffix specified.")
	cmd.Flags().StringArrayVar(&opts.labelAdd, "label-add", nil,
		"Add or change a service label. Can be specified multiple times.\n"+
			"Format: key=value or just key for an empty value.")
	cmd.Flags().StringArrayVar(&opts.labelRm, "label-rm", nil,
		"Remove a service label by its key. Can be specified multiple times.")

	return cmd
}

func update(ctx context.Context, uncli *cli.CLI, opts updateOptions) error {
	if !opts.cpuChanged && !opts.memoryChanged && !opts.reservationChanged &&
		len(opts.labelAdd) == 0 && len(opts.labelRm) == 0 {
		return errors.New("no changes specified, set at least one of the flags: " +
			"--cpu, --memory, --memory-reservation, --label-add, --label-rm")
	}

	labelAdd, err := parseLabels(opts.labelAdd)
	if err != nil {
		return err
	}
	if err = api.ValidateLabels(labelAdd); err != nil {
		return fmt.Errorf("invalid service labels: %w", err)
	}
	for _, key := range opts.labelRm {
		if _, ok := labelAdd[key]; ok {
			return fmt.Errorf("label '%s' cannot be both added and removed", key)
		}
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

	// Check all containers before changing any of them to avoid a partially updated service when a limit can't be
	// changed in place. Containers may have different specs after a failed deployment so each one is updated from
	// its own spec.
	updates := make(map[string]api.UpdateContainerOptions, len(svc.Containers))
	for _, mc := range svc.Containers {
		spec := applyServiceUpdate(mc.Container.ServiceSpec, opts, labelAdd)

		resources := mc.Container.AppliedResources()
		if err = resources.ValidateUpdateInPlace(spec.Container.Resources); err != nil {
			return fmt.Errorf("container '%s' on machine '%s' can't be updated without recreating it: %w",
				stringid.TruncateID(mc.Container.ID), mc.MachineName, err)
		}

		containerOpts := deploy.ContainerUpdateOptions(mc.Container, spec)
		if containerOpts != (api.UpdateContainerOptions{}) {
			updates[mc.Container.ID] = containerOpts
		}
	}

	if len(updates) == 0 {
		fmt.Printf("Service %s is already up to date. No changes required.\n", tui.NameStyle.Render(svc.Name))
		return nil
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		// Keep updating the remaining containers if one fails, for example, because its machine is down.
		// Running the command again only updates the containers that still differ.
		var errs []error
		for id, containerOpts := range updates {
			if err := clusterClient.UpdateContainer(ctx, svc.ID, id, containerOpts); err != nil {
				errs = append(errs, fmt.Errorf("update container '%s': %w", stringid.TruncateID(id), err))
			}
		}
		return errors.Join(errs...)
	}, uncli.ProgressOut(), "Updating service "+tui.NameStyle.Render(svc.Name))
}

// applyServiceUpdate returns a copy of the spec with the requested changes applied.
func applyServiceUpdate(spec api.ServiceSpec, opts updateOptions, labelAdd map[string]string) api.ServiceSpec {
	updated := spec.Clone()

	if opts.cpuChanged {
		updated.Container.Resources.CPU = opts.cpu.Value()
	}
	if opts.memoryChanged {
		updated.Container.Resources.Memory = opts.memory.Value()
	}
	if opts.reservationChanged {
		updated.Container.Resources.MemoryReservation = opts.memoryReservation.Value()
	}

	if len(labelAdd) > 0 && updated.Labels == nil {
		updated.Labels = make(map[string]string, len(labelAdd))
	}
	maps.Copy(updated.Labels, labelAdd)

	for key := range updated.Labels {
		if slices.Contains(opts.labelRm, key) {
			delete(updated.Labels, key)
		}
	}

	return updated
}
