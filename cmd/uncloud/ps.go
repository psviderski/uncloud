package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"

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
		Short: "List all service containers in the cluster",
		Long: `List all service containers across all machines in the cluster.

This command provides a comprehensive overview of all running containers that are part of a service,
making it easy to see the distribution and status of containers across the cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.sortBy != sortByService && opts.sortBy != sortByMachine && opts.sortBy != sortByHealth {
				return fmt.Errorf("invalid value for --sort: %q, must be one of '%s', '%s' or '%s'", opts.sortBy, sortByService, sortByMachine, sortByHealth)
			}
			return runPs(cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.sortBy, "sort", "s", sortByService, "Sort containers by 'service', 'machine' or 'health'")
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
}

func runPs(cmd *cobra.Command, opts psOptions) error {
	uncli := cmd.Context().Value("cli").(*cli.CLI)
	client, err := uncli.ConnectCluster(cmd.Context())
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	var containers []containerInfo
	err = spinner.New().
		Title(" Collecting container info...").
		Type(spinner.MiniDot).
		Style(lipgloss.NewStyle().Foreground(lipgloss.Color("3"))).
		ActionWithErr(func(ctx context.Context) error {
			containers, err = collectContainers(ctx, client)
			return err
		}).
		Run()
	if err != nil {
		return fmt.Errorf("collect containers: %w", err)
	}

	// Sort the containers based on the sorting option
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
			if a.machineName != b.machineName {
				return a.machineName < b.machineName
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
			if a.machineName != b.machineName {
				return a.machineName < b.machineName
			}
		}
		// Final tie-breaker
		return a.name < b.name
	})

	return printContainers(os.Stdout, containers)
}

func printContainers(out io.Writer, containers []containerInfo) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "SERVICE\tCONTAINER ID\tNAME\tIMAGE\tSTATUS\tMACHINE")

	for _, ctr := range containers {
		id := ctr.id
		if len(id) > 12 {
			id = id[:12]
		}

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

		fmt.Fprintf(
			w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ctr.serviceName, id, ctr.name, ctr.image,
			statusStyle.Render(ctr.status),
			ctr.machineName,
		)
	}
	return nil
}

func collectContainers(ctx context.Context, cli *client.Client) ([]containerInfo, error) {
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	// List all available machines and create a list of IPs and an IP to name map
	machinesNamesByIP := make(map[string]string)
	md := metadata.New(nil)
	for _, m := range machines {
		if addr, err := m.Machine.Network.ManagementIp.ToAddr(); err == nil {
			machineIP := addr.String()
			machinesNamesByIP[machineIP] = m.Machine.Name
			md.Append("machines", machineIP)
		}
	}
	listCtx := metadata.NewOutgoingContext(ctx, md)

	// List all service containers across all machines in the cluster.
	machineContainers, err := cli.Docker.ListServiceContainers(
		listCtx, "", container.ListOptions{All: true},
	)
	if err != nil {
		return nil, fmt.Errorf("list service containers: %w", err)
	}

	var containers []containerInfo
	for _, msc := range machineContainers {
		if msc.Metadata == nil {
			continue
		}
		if msc.Metadata.Error != "" {
			return nil, fmt.Errorf("list containers on machine %s: %s", msc.Metadata.Machine, msc.Metadata.Error)
		}

		machineName, ok := machinesNamesByIP[msc.Metadata.Machine]
		if !ok {
			machineName = msc.Metadata.Machine
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

			info := containerInfo{
				serviceName: ctr.ServiceName(),
				machineName: machineName,
				id:          ctr.Container.ID,
				name:        ctr.Container.Name,
				image:       ctr.Container.Config.Image,
				status:      status,
				highlight:   highlight,
			}
			containers = append(containers, info)
		}
	}
	return containers, nil
}
