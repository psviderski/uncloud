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
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	noReset bool
	yes     bool
}

func NewRmCommand() *cobra.Command {
	opts := removeOptions{}

	cmd := &cobra.Command{
		Use:     "rm MACHINE",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove a machine from a cluster and reset it.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args[0], opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Do not prompt for confirmation before removing the machine.")
	cmd.Flags().BoolVar(&opts.noReset, "no-reset", false,
		"Do not reset the machine after removing it from the cluster. This will leave all containers and data intact.")

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, nameOrID string, opts removeOptions) error {
	// TODO: automatically choose a connection to the machine that is not being removed.
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Verify the machine exists and list all service containers on it including stopped ones.
	mctx, machines, err := client.ProxyMachinesContext(ctx, []string{nameOrID})
	if err != nil {
		return err
	}
	if len(machines) == 0 {
		return fmt.Errorf("machine '%s' not found in the cluster", nameOrID)
	}
	m := machines[0].Machine

	// Verify if the machine being removed is the proxy machine we're connected to.
	proxyMachine, err := client.MachineClient.Inspect(ctx, nil)
	if err != nil {
		return fmt.Errorf("inspect proxy machine: %w", err)
	}
	if proxyMachine.Id == m.Id {
		allMachines, err := client.ListMachines(ctx, nil)
		if err != nil {
			return fmt.Errorf("list machines: %w", err)
		}
		if len(allMachines) > 1 {
			return errors.New("cannot remove the machine you are currently connected to. " +
				"Please connect to another machine in the cluster and try again. " +
				"Use --connect flag or update 'connections' for the cluster context in your Uncloud config")
			// It's ok to remove the proxy machine if it's the last one in the cluster.
		}
	}

	// TODO: mark the machine as being removed and unschedulable when this is possible to prevent new containers
	//  from being scheduled on it while the removal is in progress.

	reset := !opts.noReset
	var containers []api.ServiceContainer
	reachable := false
	if reset {
		// Check if the machine is up and has service containers.
		listOpts := container.ListOptions{All: true}
		machineContainers, err := client.Docker.ListServiceContainers(mctx, "", listOpts)
		if err == nil {
			reachable = true
			containers = machineContainers[0].Containers
			if len(containers) > 0 {
				plural := ""
				if len(containers) > 1 {
					plural = "s"
				}
				fmt.Printf("Found %d service container%s on machine '%s':\n", len(containers), plural, m.Name)
				fmt.Println(formatContainerTree(containers))
				fmt.Println()
				fmt.Println("This will remove all service containers from the machine, remove it from the cluster, " +
					"and reset it to the uninitialised state.")
			} else {
				fmt.Printf("No service containers found on machine '%s'.\n", m.Name)
				fmt.Println("This will remove the machine from the cluster and reset it to the uninitialised state.")
			}
		} else {
			fmt.Printf("This will remove machine '%s' from the cluster without resetting it as it's unreachable.\n",
				m.Name)
		}
	} else {
		fmt.Printf("This will remove machine '%s' from the cluster without resetting it.\n", m.Name)
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

	if reset && len(containers) > 0 {
		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			return removeContainers(ctx, client, containers)
		}, uncli.ProgressOut(), "Removing containers")
		if err != nil {
			return fmt.Errorf("remove containers: %w", err)
		}
		fmt.Println()
	}

	if _, err = client.RemoveMachine(ctx, &pb.RemoveMachineRequest{Id: m.Id}); err != nil {
		return fmt.Errorf("remove machine from cluster: %w", err)
	}
	fmt.Printf("Machine '%s' removed from the cluster.\n", m.Name)

	if reset && reachable {
		_, err = client.MachineClient.Reset(mctx, &pb.ResetRequest{})
		if err != nil {
			fmt.Printf("WARNING: Failed to reset machine: %v\n", err)
		} else {
			fmt.Println("Machine reset initiated and will complete in the background.")
		}
	}

	// Remove the connection to the machine from the uncloud config if it exists.
	if uncli.Config != nil {
		contextName := uncli.GetContextOverrideOrCurrent()
		if context, ok := uncli.Config.Contexts[contextName]; ok {
			for i, c := range context.Connections {
				if c.MachineID == m.Id {
					context.Connections = slices.Delete(context.Connections, i, i+1)
					break
				}
			}
			if err := uncli.Config.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
		}
	}

	// TODO: If Caddy was running on this machine and a cluster domain is reserved,
	//  let the user know that the DNS records should be updated.

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
