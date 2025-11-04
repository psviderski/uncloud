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
)

const (
	statusHealthy   = "healthy"
	statusUnhealthy = "unhealthy"
	statusRunning   = "running"
	statusOther     = "other"
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
			if opts.groupBy != groupByService && opts.groupBy != groupByMachine {
				return fmt.Errorf("invalid value for --group-by: %q, must be one of '%s' or '%s'", opts.groupBy, groupByService, groupByMachine)
			}
			return runPs(cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.groupBy, "group-by", "g", groupByService, "Group containers by 'service' or 'machine'")
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
	statusState string
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
		if opts.groupBy == groupByMachine {
			if containers[i].machineName != containers[j].machineName {
				return containers[i].machineName < containers[j].machineName
			}
			// If machine names are the same, then sort by service name
			if containers[i].serviceName != containers[j].serviceName {
				return containers[i].serviceName < containers[j].serviceName
			}
		} else { // opts.groupBy == groupByService
			if containers[i].serviceName != containers[j].serviceName {
				return containers[i].serviceName < containers[j].serviceName
			}
			// If service names are the same, then sort by machine name
			if containers[i].machineName != containers[j].machineName {
				return containers[i].machineName < containers[j].machineName
			}
		}
		// Finally, sort by container name if both machine and service names are the same
		return containers[i].name < containers[j].name
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
		switch ctr.statusState {
		case statusHealthy:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
		case statusUnhealthy:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // Red
		case statusOther:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // Yellow
		default: // statusRunning
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

			var statusState string
			healthStatus := ""
			if ctr.Container.State.Health != nil {
				healthStatus = ctr.Container.State.Health.Status
			}

			if healthStatus == container.Healthy {
				statusState = statusHealthy
			} else if healthStatus == container.Unhealthy {
				statusState = statusUnhealthy
			} else if ctr.Container.State.Status == "running" {
				statusState = statusRunning
			} else {
				statusState = statusOther
			}

			info := containerInfo{
				serviceName: service.Name,
				machineName: machinesNamesByID[ctr.MachineID],
				id:          ctr.Container.ID,
				name:        ctr.Container.Name,
				image:       ctr.Container.Config.Image,
				status:      status,
				statusState: statusState,
			}
			containers = append(containers, info)
		}
	}
	return containersCollectedMsg{containers: containers}
}
