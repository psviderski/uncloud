package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/completion"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	service string
}

func NewInspectCommand(groupID string) *cobra.Command {
	opts := inspectOptions{}
	cmd := &cobra.Command{
		Use:   "inspect SERVICE",
		Short: "Display detailed information on a service.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.service = args[0]
			return inspect(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Services(cmd.Context(), uncli, args, toComplete)
		},
	}
	return cmd
}

func inspect(ctx context.Context, uncli *cli.CLI, opts inspectOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	svc, err := client.InspectService(ctx, opts.service)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machinesNamesByID := make(map[string]string)
	for _, m := range machines {
		machinesNamesByID[m.Machine.Id] = m.Machine.Name
	}

	fmt.Printf("Service ID: %s\n", svc.ID)
	fmt.Printf("Name:       %s\n", svc.Name)
	fmt.Printf("Mode:       %s\n", svc.Mode)
	fmt.Println()

	// Combine regular and hook containers.
	allContainers := append(svc.Containers, svc.HookContainers...)

	// Parse created times for sorting and display.
	createdTimes := make(map[string]time.Time, len(allContainers))
	for _, ctr := range allContainers {
		createdTimes[ctr.Container.ID], _ = time.Parse(time.RFC3339Nano, ctr.Container.Created)
	}

	// Sort containers by created time (newest first).
	slices.SortFunc(allContainers, func(a, b api.MachineServiceContainer) int {
		return createdTimes[b.Container.ID].Compare(createdTimes[a.Container.ID])
	})

	// Print the list of containers in a table format.
	// Show HOOK column only when hook containers are present.
	hasHooks := len(svc.HookContainers) > 0

	t := tui.NewTable()
	if hasHooks {
		t.Headers("CONTAINER ID", "IMAGE", "CREATED", "STATUS", "HOOK", "IP ADDRESS", "MACHINE")
	} else {
		t.Headers("CONTAINER ID", "IMAGE", "CREATED", "STATUS", "IP ADDRESS", "MACHINE")
	}

	now := time.Now().UTC()
	for _, ctr := range allContainers {
		created := units.HumanDuration(now.Sub(createdTimes[ctr.Container.ID])) + " ago"

		machine := machinesNamesByID[ctr.MachineID]
		if machine == "" {
			machine = ctr.MachineID
		}
		state, err := ctr.Container.HumanState()
		if err != nil {
			return fmt.Errorf("get human state: %w", err)
		}

		ip := ctr.Container.UncloudNetworkIP()
		ipStr := ""
		// The container might not have an IP if it's not running or uses the host network.
		if ip.IsValid() {
			ipStr = ip.String()
		}

		if hasHooks {
			t.Row(
				stringid.TruncateID(ctr.Container.ID),
				tui.FormatImage(ctr.Container.Config.Image, tui.NoStyle),
				created,
				state,
				ctr.Container.Config.Labels[api.LabelHook],
				ipStr,
				machine,
			)
		} else {
			t.Row(
				stringid.TruncateID(ctr.Container.ID),
				tui.FormatImage(ctr.Container.Config.Image, tui.NoStyle),
				created,
				state,
				ipStr,
				machine,
			)
		}
	}

	fmt.Println(t)
	return nil
}
