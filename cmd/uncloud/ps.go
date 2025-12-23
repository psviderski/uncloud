package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client"
)

const (
	sortByService = "service"
	sortByMachine = "machine"
	sortByHealth  = "health"
)

type containerHighlight int

const (
	highlightDanger containerHighlight = iota
	highlightWarning
	highlightSuccess
	highlightNormal
)

type psOptions struct {
	sortBy string
}

func NewPsCommand() *cobra.Command {
	opts := psOptions{}
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List all service containers.",
		Long: `List all service containers across all machines in the cluster.

This command provides a comprehensive overview of all running containers that are part of a service,
making it easy to see the distribution and status of containers across the cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if opts.sortBy != sortByService && opts.sortBy != sortByMachine && opts.sortBy != sortByHealth {
				return fmt.Errorf("invalid value for --sort: %q, must be one of '%s', '%s' or '%s'", opts.sortBy,
					sortByService, sortByMachine, sortByHealth)
			}

			return runPs(cmd.Context(), uncli, opts)
		},
		GroupID: "service",
	}
	cmd.Flags().StringVarP(&opts.sortBy, "sort", "s", sortByService,
		"Sort containers by 'service', 'machine', or 'health'.")
	return cmd
}

type containerInfo struct {
	serviceName string
	machineName string
	id          string
	name        string
	image       string
	status      string
	highlight   containerHighlight
	created     time.Time
	ip          string
}

func runPs(ctx context.Context, uncli *cli.CLI, opts psOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	var containers []containerInfo
	err = spinner.New().
		Title(" Collecting container info...").
		Type(spinner.MiniDot).
		Style(lipgloss.NewStyle().Foreground(lipgloss.Color("3"))).
		ActionWithErr(func(ctx context.Context) error {
			containers, err = collectContainers(ctx, clusterClient)
			return err
		}).
		Run()
	if err != nil {
		return fmt.Errorf("collect containers: %w", err)
	}

	// Sort the containers based on the sorting option.
	sort.SliceStable(containers, func(i, j int) bool {
		a, b := containers[i], containers[j]
		switch opts.sortBy {
		case sortByHealth:
			if a.highlight != b.highlight {
				return a.highlight < b.highlight
			}
			if a.serviceName != b.serviceName {
				return a.serviceName < b.serviceName
			}
		case sortByMachine:
			if a.machineName != b.machineName {
				return a.machineName < b.machineName
			}
			if a.serviceName != b.serviceName {
				return a.serviceName < b.serviceName
			}
		default: // sortByService
			if a.serviceName != b.serviceName {
				return a.serviceName < b.serviceName
			}
		}
		// Fallback to creation time (newest first).
		return a.created.After(b.created)
	})

	return printContainers(containers)
}

func printContainers(containers []containerInfo) error {
	t := table.New().
		// Remove the default border.
		Border(lipgloss.Border{}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingRight(3)
			}
			// Regular style for data rows with padding.
			return lipgloss.NewStyle().PaddingRight(3)
		})

	t.Headers("SERVICE", "CONTAINER ID", "CONTAINER NAME", "IMAGE", "CREATED", "STATUS", "IP ADDRESS", "MACHINE")

	for _, ctr := range containers {
		id := ctr.id
		if len(id) > 12 {
			id = id[:12]
		}

		created := units.HumanDuration(time.Now().UTC().Sub(ctr.created)) + " ago"

		var statusStyle lipgloss.Style
		switch ctr.highlight {
		case highlightSuccess:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
		case highlightDanger:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // Red
		case highlightWarning:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // Yellow
		default:
			statusStyle = lipgloss.NewStyle() // Default
		}

		t.Row(
			ctr.serviceName,
			id,
			ctr.name,
			ctr.image,
			created,
			statusStyle.Render(ctr.status),
			ctr.ip,
			ctr.machineName,
		)
	}

	fmt.Println(t)
	return nil
}

func collectContainers(ctx context.Context, cli *client.Client) ([]containerInfo, error) {
	listCtx, machines, err := cli.ProxyMachinesContext(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy machines context: %w", err)
	}

	// Create a map of IP to machine name for resolving response metadata
	machinesNamesByIP := make(map[string]string)
	for _, m := range machines {
		if addr, err := m.Machine.Network.ManagementIp.ToAddr(); err == nil {
			machinesNamesByIP[addr.String()] = m.Machine.Name
		}
	}

	// List all service containers across all machines in the cluster.
	machineContainers, err := cli.Docker.ListServiceContainers(
		listCtx, "", container.ListOptions{All: true},
	)
	if err != nil {
		return nil, fmt.Errorf("list service containers: %w", err)
	}

	var containers []containerInfo
	for _, msc := range machineContainers {
		// Metadata can be nil if the request was broadcasted to only one machine.
		if msc.Metadata == nil && len(machineContainers) > 1 {
			return nil, fmt.Errorf("something went wrong with gRPC proxy: metadata is missing for a machine response")
		}

		machineName := "unknown"
		if msc.Metadata != nil {
			var ok bool
			machineName, ok = machinesNamesByIP[msc.Metadata.Machine]
			if !ok {
				// Fallback to machine's IP as name.
				machineName = msc.Metadata.Machine
			}
		} else {
			// Fallback to the first available machine name.
			if len(machines) > 0 {
				machineName = machines[0].Machine.Name
			}
		}

		if msc.Metadata != nil && msc.Metadata.Error != "" {
			client.PrintWarning(fmt.Sprintf("failed to list containers on machine %s: %s", machineName,
				msc.Metadata.Error))
			continue
		}

		for _, ctr := range msc.Containers {
			if ctr.Container.State == nil || ctr.Container.Config == nil {
				continue
			}

			status, err := ctr.Container.HumanState()
			if err != nil {
				return nil, fmt.Errorf("get human state for container %s: %w", ctr.Container.ID, err)
			}

			var highlight containerHighlight
			healthStatus := ""
			if ctr.Container.State.Health != nil {
				healthStatus = ctr.Container.State.Health.Status
			}

			if healthStatus == container.Unhealthy || ctr.Container.State.Status == "dead" || ctr.Container.State.OOMKilled || ctr.Container.State.Dead {
				highlight = highlightDanger
			} else if healthStatus == container.Healthy {
				highlight = highlightSuccess
			} else if ctr.Container.State.Status == "running" {
				highlight = highlightNormal
			} else { // Other non-critical but noteworthy states
				highlight = highlightWarning
			}

			created, _ := time.Parse(time.RFC3339Nano, ctr.Container.Created)

			ip := ctr.Container.UncloudNetworkIP()
			ipStr := ""
			// The container might not have an IP if it's not running or uses the host network.
			if ip.IsValid() {
				ipStr = ip.String()
			}

			info := containerInfo{
				serviceName: ctr.ServiceName(),
				machineName: machineName,
				id:          ctr.Container.ID,
				name:        ctr.Container.Name,
				image:       ctr.Container.Config.Image,
				status:      status,
				highlight:   highlight,
				created:     created,
				ip:          ipStr,
			}
			containers = append(containers, info)
		}
	}
	return containers, nil
}
