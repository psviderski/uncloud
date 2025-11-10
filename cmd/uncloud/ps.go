package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client"
)

const (
	groupByService = "service"
	groupByMachine = "machine"
	groupByHealth  = "health"
)

type containerHighlight int

const (
	highlightDanger containerHighlight = iota
	highlightWarning
	highlightSuccess
	highlightNormal
)

type psOptions struct {
	groupBy     string
	contextName string
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
			if opts.groupBy != groupByService && opts.groupBy != groupByMachine && opts.groupBy != groupByHealth {
				return fmt.Errorf("invalid value for --group-by: %q, must be one of '%s', '%s' or '%s'", opts.groupBy, groupByService, groupByMachine, groupByHealth)
			}
			return runPs(cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.groupBy, "group-by", "g", groupByService, "Group containers by 'service', 'machine' or 'health'")
	cmd.Flags().StringVarP(&opts.contextName, "context", "c", "", "Name of the cluster context. (default is the current context)")
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
	client, err := uncli.ConnectCluster(cmd.Context(), opts.contextName)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	model := newSpinnerModel(client, "Collecting container info...")
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("failed to run spinner: %w", err)
	}

	m := finalModel.(spinnerModel)
	if m.err != nil {
		return m.err
	}

	containers := m.containers
	// Sort the containers based on the grouping option
	sort.SliceStable(containers, func(i, j int) bool {
		a, b := containers[i], containers[j]
		switch opts.groupBy {
		case groupByHealth:
			if a.highlight != b.highlight {
				return a.highlight < b.highlight
			}
			if a.serviceName != b.serviceName {
				return a.serviceName < b.serviceName
			}
			if a.machineName != b.machineName {
				return a.machineName < b.machineName
			}
		case groupByMachine:
			if a.machineName != b.machineName {
				return a.machineName < b.machineName
			}
			if a.serviceName != b.serviceName {
				return a.serviceName < b.serviceName
			}
		default: // groupByService
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

	return printContainers(os.Stdout, containers, opts.groupBy)
}

func printContainers(out io.Writer, containers []containerInfo, groupBy string) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	defer w.Flush()

	var header string
	if groupBy == groupByMachine {
		header = "MACHINE\tSERVICE\tCONTAINER ID\tNAME\tIMAGE\tSTATUS"
	} else {
		header = "SERVICE\tMACHINE\tCONTAINER ID\tNAME\tIMAGE\tSTATUS"
	}
	fmt.Fprintln(w, header)

	for _, ctr := range containers {
		id := ctr.id
		if len(id) > 12 {
			id = id[:12]
		}
		name := strings.TrimPrefix(ctr.name, "/")

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

		var row string
		if groupBy == groupByMachine {
			row = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s",
				ctr.machineName, ctr.serviceName, id, name, ctr.image, statusStyle.Render(ctr.status))
		} else {
			row = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s",
				ctr.serviceName, ctr.machineName, id, name, ctr.image, statusStyle.Render(ctr.status))
		}
		fmt.Fprintln(w, row)
	}
	return nil
}

type spinnerModel struct {
	client     *client.Client
	spinner    spinner.Model
	message    string
	containers []containerInfo
	err        error
}

type containersCollectedMsg struct {
	containers []containerInfo
	err        error
}

func newSpinnerModel(client *client.Client, message string) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Jump
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return spinnerModel{client: client, spinner: s, message: message}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.collectContainers)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case containersCollectedMsg:
		m.containers = msg.containers
		m.err = msg.err
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m spinnerModel) View() string {
	return fmt.Sprintf("%s %s", m.spinner.View(), m.message)
}

func (m spinnerModel) collectContainers() tea.Msg {
	services, err := m.client.ListServices(context.Background())
	if err != nil {
		return containersCollectedMsg{err: fmt.Errorf("list services: %w", err)}
	}

	var containers []containerInfo
	for _, s := range services {
		service, err := m.client.InspectService(context.Background(), s.ID)
		if err != nil {
			return containersCollectedMsg{err: fmt.Errorf("inspecting service %q (%s): %w", s.Name, s.ID, err)}
		}

		machines, err := m.client.ListMachines(context.Background(), nil)
		if err != nil {
			return containersCollectedMsg{err: fmt.Errorf("list machines: %w", err)}
		}
		machinesNamesByID := make(map[string]string)
		for _, m := range machines {
			machinesNamesByID[m.Machine.Id] = m.Machine.Name
		}

		for _, ctr := range service.Containers {
			status, err := ctr.Container.HumanState()
			if err != nil {
				return containersCollectedMsg{err: fmt.Errorf("get human state for container %s: %w", ctr.Container.ID, err)}
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
				serviceName: service.Name,
				machineName: machinesNamesByID[ctr.MachineID],
				id:          ctr.Container.ID,
				name:        ctr.Container.Name,
				image:       ctr.Container.Config.Image,
				status:      status,
				highlight:   highlight,
			}
			containers = append(containers, info)
		}
	}
	return containersCollectedMsg{containers: containers}
}
