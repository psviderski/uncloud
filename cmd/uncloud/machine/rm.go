package machine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/docker/compose/v2/pkg/progress"
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
		plural := ""
		if len(containers) > 1 {
			plural = "s"
		}
		fmt.Printf("Found %d service container%s on machine '%s':\n", len(containers), plural, m.Name)
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

	if len(containers) > 0 {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			return removeContainers(ctx, client, containers)
		}, uncli.ProgressOut(), "Removing containers")

		if err != nil {
			return fmt.Errorf("remove containers: %w", err)
		}
		fmt.Println()
	}

	// TODO: 4. Implement and call Reset via Machine API to reset the machine state to uninitialised.
	// TODO: 5. Remove the machine from the cluster store.

	return fmt.Errorf("resetting machine is not fully implemented yet")
	//fmt.Printf("Machine '%s' removed from the cluster.\n", m.Name)
	//return nil
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

// removeContainers removes the given service containers from the machine.
func removeContainers(ctx context.Context, client api.Client, containers []api.ServiceContainer) error {
	if len(containers) == 0 {
		return nil
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error)

	for _, ctr := range containers {
		wg.Add(1)
		go func(c api.ServiceContainer) {
			defer wg.Done()

			// Gracefully stop the container before removing it.
			err := client.StopContainer(ctx, c.ServiceID(), c.ID, container.StopOptions{})
			if err != nil && !errors.Is(err, api.ErrNotFound) {
				errCh <- fmt.Errorf("stop container '%s': %w", c.ID, err)
			}

			err = client.RemoveContainer(ctx, c.ServiceID(), c.ID, container.RemoveOptions{
				// Remove anonymous volumes created by the container.
				RemoveVolumes: true,
			})
			if err != nil && !errors.Is(err, api.ErrNotFound) {
				errCh <- fmt.Errorf("remove container '%s': %w", c.ID, err)
			}
		}(ctr)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var err error
	for e := range errCh {
		err = errors.Join(err, e)
	}

	return err
}
