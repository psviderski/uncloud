package machine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/config"
	"github.com/spf13/cobra"
)

type contextOptions struct {
	context string
	sshKey  string
	write   bool
}

func NewContextCommand() *cobra.Command {
	opts := contextOptions{}
	cmd := &cobra.Command{
		Aliases: []string{"context"},
		Use:     "ctx [schema://]USER@HOST[:PORT]",
		Short:   "Add the cluster context to Uncloud configuration file by connecting to the remote machine.",
		Long: `Add the cluster context, or add new machines to an existing cluster context.
This command adds or updates an (existing) context in your Uncloud config.

Connection methods:
  [ssh://]user@host   - Use system 'ssh' command with full SSH config support (default, no prefix required)
  ssh+go://user@host  - Use Go's built-in SSH library`,
		Example: `  # Get the cluster context with default settings.
  uc machine ctx -w root@<your-server-ip>

  # Add a new context named 'prod' in the Uncloud config (~/.config/uncloud/config.yaml).
  uc machine ctx -w root@<your-server-ip> -c prod

  # Add a new context with a non-root user and custom SSH port and key.
  uc machine ctx -w ubuntu@<your-server-ip>:2222 -i ~/.ssh/mykey`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			destination := args[0]
			useSSHGo := strings.HasPrefix(destination, "ssh+go://")
			destination = strings.TrimPrefix(destination, "ssh+go://")
			destination = strings.TrimPrefix(destination, "ssh+cli://")
			destination = strings.TrimPrefix(destination, "ssh://")

			if _, _, _, err := config.SSHDestination(destination).Parse(); err != nil {
				return fmt.Errorf("parse remote machine: %w", err)
			}

			conn := config.MachineConnection{}
			if useSSHGo {
				conn.SSHGo = config.SSHDestination(destination)
			} else {
				conn.SSH = config.SSHDestination(destination)
			}

			return listContext(cmd.Context(), uncli, conn, opts)
		},
	}

	cmd.Flags().StringVarP(
		&opts.context, "context", "c", cli.DefaultContextName,
		"Name of the new context to be created in the Uncloud config to manage the cluster.",
	)
	cmd.Flags().StringVarP(
		&opts.sshKey, "ssh-key", "i", "",
		fmt.Sprintf("Path to SSH private key for remote login (if not already added to SSH agent). (default %q)",
			cli.DefaultSSHKeyPath),
	)
	cmd.Flags().BoolVarP(
		&opts.write, "write", "w", false,
		"Write a new Uncloud config, by default the config is only printed to standard output.",
	)

	return cmd
}

func listContext(ctx context.Context, uncli *cli.CLI, conn config.MachineConnection, opts contextOptions) error {
	contextName, err := uncli.NewContextName(opts.context)
	if err != nil {
		return err
	}

	conn.SSHKeyFile = opts.sshKey
	client, err := cli.ConnectCluster(ctx, conn, cli.ConnectOptions{ShowProgress: true})
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Figure out if one of the machines is already in a context, and add the remaining there. Otherwise we
	// create a new context with the optional name we got from the command line.
	for name, context := range uncli.Config.Contexts {
		for _, conn := range context.Connections {
			for _, machine := range machines {
				if machine.Machine.Id == conn.MachineID {
					contextName = name
					break
				}
			}
		}
	}

	var (
		user string
		port int
	)

	if conn.SSH != "" {
		user, _, port, _ = conn.SSH.Parse()
	}
	if conn.SSHGo != "" {
		user, _, port, _ = conn.SSHGo.Parse()
	}

	connCfg := []config.MachineConnection{}
	for _, machine := range machines {
		addr, _ := machine.Machine.PublicIp.ToAddr()
		dest := config.NewSSHDestination(user, addr.String(), port)

		machineConn := config.MachineConnection{MachineID: machine.Machine.Id}
		if conn.SSH != "" {
			machineConn.SSH = dest
		}
		if conn.SSHGo != "" {
			machineConn.SSHGo = dest
		}
		connCfg = append(connCfg, machineConn)
	}

	if !opts.write {
		encoder := yaml.NewEncoder(os.Stdout, yaml.Indent(2), yaml.IndentSequence(true))
		contexts := map[string]*config.Context{
			contextName: {
				Connections: connCfg,
			},
		}

		encoder.Encode(contexts)
		return nil
	}

	uncli.Config.Contexts[contextName].Connections = connCfg
	if err = uncli.Config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
