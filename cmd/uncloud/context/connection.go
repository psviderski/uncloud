package context

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/spf13/cobra"
)

func NewConnectionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Choose a new default connection for the current context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return selectConnection(uncli)
		},
	}
	return cmd
}

func selectConnection(uncli *cli.CLI) error {
	if uncli.Config == nil {
		return fmt.Errorf("connection management is not available: Uncloud configuration file is not being used")
	}
	if len(uncli.Config.Contexts) == 0 {
		return fmt.Errorf("no contexts found in Uncloud config (%s)", uncli.Config.Path())
	}

	currentCtxName := uncli.Config.CurrentContext
	currentCtx, ok := uncli.Config.Contexts[currentCtxName]
	if !ok {
		return fmt.Errorf("current context '%s' not found", currentCtxName)
	}

	if len(currentCtx.Connections) == 0 {
		return fmt.Errorf("no connections found in context '%s'", currentCtxName)
	}

	var selectedConnection *config.MachineConnection
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[*config.MachineConnection]().
				Title("Select a default connection").
				Options(buildConnectionOptions(currentCtx.Connections)...).
				Value(&selectedConnection),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("select connection: %w", err)
	}

	// Reorder the connections with the selected one at the top.
	newConnections := make([]config.MachineConnection, 0, len(currentCtx.Connections))
	newConnections = append(newConnections, *selectedConnection)
	for _, conn := range currentCtx.Connections {
		if !conn.Equal(*selectedConnection) {
			newConnections = append(newConnections, conn)
		}
	}
	currentCtx.Connections = newConnections

	if err := uncli.Config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Default connection for context '%s' is now '%s'.\n", currentCtxName, selectedConnection.String())
	return nil
}

func buildConnectionOptions(connections []config.MachineConnection) []huh.Option[*config.MachineConnection] {
	options := make([]huh.Option[*config.MachineConnection], len(connections))
	for i, conn := range connections {
		key := conn.String()
		opt := huh.NewOption(key, &connections[i])
		if i == 0 {
			opt.Key += " (default)"
			opt = opt.Selected(true)
		}
		options[i] = opt
	}
	return options
}
