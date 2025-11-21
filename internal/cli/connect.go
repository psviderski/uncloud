package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/connector"
)

// ConnectOptions configures the behavior of cluster connection attempts.
type ConnectOptions struct {
	// Whether to show connection progress spinner if stdout is a terminal or progress logs if not.
	ShowProgress bool
}

func ConnectCluster(ctx context.Context, conn config.MachineConnection, opts ConnectOptions) (*client.Client, error) {
	if opts.ShowProgress {
		return connectClusterWithProgress(ctx, conn)
	}
	return connectCluster(ctx, conn)
}

// connectClusterWithProgress connects to the cluster while displaying a progress spinner.
// If the stdout is not a terminal, it falls back to simple progress logs to stderr.
func connectClusterWithProgress(ctx context.Context, conn config.MachineConnection) (*client.Client, error) {
	// If stdout is not a terminal, fall back to simple progress logs.
	if !IsStdoutTerminal() {
		fmt.Fprintln(os.Stderr, "Connecting to", conn.String())
		cli, err := connectCluster(ctx, conn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Connection failed:", err)
		} else {
			fmt.Fprintln(os.Stderr, "Connected to cluster.")
		}
		return cli, err
	}

	// Run the connection TUI model.
	p := tea.NewProgram(newConnectModel(ctx, conn))
	model, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run connection TUI: %w", err)
	}

	m := model.(connectModel)
	return m.result.client, m.result.err
}

func connectCluster(ctx context.Context, conn config.MachineConnection) (*client.Client, error) {
	// Determine which SSH type is configured
	var sshDest config.SSHDestination
	var useSSHCLI bool

	// Validate connection configuration early to provide clear error messages.
	if err := conn.Validate(); err != nil {
		return nil, fmt.Errorf("invalid connection configuration: %w", err)
	}

	if conn.SSH != "" {
		sshDest = conn.SSH
		useSSHCLI = false
	} else if conn.SSHCLI != "" {
		sshDest = conn.SSHCLI
		useSSHCLI = true
	} else if conn.TCP != nil && conn.TCP.IsValid() {
		return client.New(ctx, connector.NewTCPConnector(*conn.TCP))
	} else if conn.Unix != "" {
		return client.New(ctx, connector.NewUnixConnector(conn.Unix))
	} else {
		return nil, errors.New("connection configuration is invalid")
	}

	// Parse SSH destination and create config (shared for both types)
	user, host, port, err := sshDest.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse SSH connection %q: %w", sshDest, err)
	}

	keyPath := fs.ExpandHomeDir(conn.SSHKeyFile)

	sshConfig := &connector.SSHConnectorConfig{
		User:    user,
		Host:    host,
		Port:    port,
		KeyPath: keyPath,
	}

	// Create appropriate connector based on type
	if useSSHCLI {
		return client.New(ctx, connector.NewSSHCLIConnector(sshConfig))
	}
	return client.New(ctx, connector.NewSSHConnector(sshConfig))
}

// connectModel is a TUI model for connecting to a cluster with a progress spinner.
type connectModel struct {
	ctx     context.Context
	conn    config.MachineConnection
	spinner spinner.Model
	// showSpinner controls whether the spinner is visible (delayed to avoid flashing).
	showSpinner bool
	// done indicates whether the connection attempt has completed (successfully or with error).
	done bool
	// result holds the result of the connection attempt.
	result connectResultMsg
}

type connectResultMsg struct {
	client *client.Client
	err    error
}

// showSpinnerMsg is sent after a delay to show the spinner.
type showSpinnerMsg struct{}

func newConnectModel(ctx context.Context, conn config.MachineConnection) connectModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // the same yellow as in compose progress

	return connectModel{
		ctx:     ctx,
		conn:    conn,
		spinner: s,
	}
}

func (m connectModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.connect(),
		m.delayShowSpinner(),
	)
}

func (m connectModel) connect() tea.Cmd {
	return func() tea.Msg {
		cli, err := connectCluster(m.ctx, m.conn)
		return connectResultMsg{
			client: cli,
			err:    err,
		}
	}
}

// delayShowSpinner returns a command that sends a message to show the spinner after a delay.
// This avoids flashing the spinner if the connection is fast.
func (m connectModel) delayShowSpinner() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return showSpinnerMsg{}
	})
}

func (m connectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case <-m.ctx.Done():
		m.result.err = m.ctx.Err()
		m.done = true
		return m, tea.Quit
	default:
	}

	switch msg := msg.(type) {
	case connectResultMsg:
		m.result = msg
		m.done = true
		return m, tea.Quit

	case showSpinnerMsg:
		// Only show spinner if connection hasn't completed yet.
		if !m.done {
			m.showSpinner = true
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.result.err = fmt.Errorf("connection cancelled")
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m connectModel) View() string {
	// Don't show anything if done or spinner not yet visible.
	if m.done || !m.showSpinner {
		return ""
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("153"))
	return fmt.Sprintf("%s %s\n",
		m.spinner.View(),
		fmt.Sprintf("Connecting to %s", style.Render(m.conn.String())),
	)
}
