package machine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	force   bool
	yes     bool
	context string
}

func NewRmCommand() *cobra.Command {
	opts := removeOptions{}

	cmd := &cobra.Command{
		Use:     "rm MACHINE",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove a machine from a cluster.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args[0], opts)
		},
	}

	cmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Do not prompt for confirmation before removing the machine.")

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, machineName string, opts removeOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Verify the machine exists and list all service containers on it including stopped ones.
	listCtx, machines, err := api.ProxyMachinesContext(ctx, client, []string{machineName})
	if err != nil {
		return err
	}
	if len(machines) == 0 {
		return fmt.Errorf("machine '%s' not found in the cluster", machineName)
	}
	m := machines[0].Machine

	listOpts := container.ListOptions{All: true}
	machineContainers, err := client.Docker.ListServiceContainers(listCtx, "", listOpts)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	containers := machineContainers[0].Containers

	if len(containers) > 0 {
		fmt.Printf("Found %d service containers on machine '%s':\n\n", len(containers), m.Name)
		fmt.Println(formatContainerTree(containers))
		fmt.Println()
		fmt.Println("This will remove all service containers on the machine, reset it to the uninitialised state, " +
			"and remove it from the cluster.")
	} else {
		fmt.Printf("No service containers found on machine '%s'.\n", m.Name)
		fmt.Println("This will reset the machine to the uninitialised state and remove it from the cluster.")
	}

	if !opts.yes {
		confirmed, err := cli.Confirm()
		if err != nil {
			return fmt.Errorf("confirm removal: %w", err)
		}
		if !confirmed {
			fmt.Println("Cancelled. Machine was not removed.")
			return nil
		}
	}

	// TODO: 3. Remove all service containers on the machine.
	// TODO: 4. Implement and call ResetMachine via Machine API to reset the machine state to uninitialised.
	// TODO: 5. Remove the machine from the cluster store.

	fmt.Printf("Machine '%s' removed from the cluster.\n", m.Name)
	return nil
}

// formatContainerTree formats a list of containers grouped by service as a tree structure.
func formatContainerTree(containers []api.ServiceContainer) string {
	if len(containers) == 0 {
		return ""
	}

	// Group containers by service.
	serviceContainers := make(map[string][]api.ServiceContainer)
	for _, ctr := range containers {
		serviceName := ctr.ServiceName()
		serviceContainers[serviceName] = append(serviceContainers[serviceName], ctr)
	}

	// Build tree output.
	var output []string
	serviceNames := slices.Sorted(maps.Keys(serviceContainers))
	for _, serviceName := range serviceNames {
		ctrs := serviceContainers[serviceName]
		mode := ctrs[0].ServiceMode()

		// Format a tree for the service with its containers.
		plural := ""
		if len(ctrs) > 1 {
			plural = "s"
		}
		t := tree.Root(fmt.Sprintf("• %s (%s, %d container%s)", serviceName, mode, len(ctrs), plural)).
			EnumeratorStyle(lipgloss.NewStyle().MarginLeft(2).MarginRight(1))

		// Add containers as children.
		for _, ctr := range ctrs {
			state, _ := ctr.HumanState()
			info := fmt.Sprintf("%s • %s • %s", ctr.Name, ctr.Config.Image, state)
			t.Child(info)
		}

		output = append(output, t.String())
	}

	return strings.Join(output, "\n")
}
