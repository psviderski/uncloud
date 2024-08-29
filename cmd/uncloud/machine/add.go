package machine

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type addOptions struct {
	name    string
	user    string
	port    int
	sshKey  string
	cluster string
}

func NewAddCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		Use:   "add HOST",
		Short: "Add a new machine to a cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return add(cmd.Context(), uncli, args[0], opts)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "Assign a name to the machine")
	cmd.Flags().StringVarP(&opts.user, "user", "u", "root", "Username for SSH remote login")
	cmd.Flags().IntVarP(&opts.port, "port", "p", 22, "Port for SSH remote login")
	cmd.Flags().StringVarP(&opts.sshKey, "ssh-key", "i", "",
		"path to SSH private key for SSH remote login (default ~/.ssh/id_*)")
	cmd.Flags().StringVarP(&opts.cluster, "cluster", "c", "",
		"Name of the cluster to add the machine to (default is the current cluster)")
	return cmd
}

func add(ctx context.Context, uncli *cli.CLI, host string, opts addOptions) error {
	var (
		cluster *cli.Cluster
		err     error
	)
	if opts.cluster == "" {
		// If the cluster is not specified, use the current cluster. If there are no clusters, create a default one.
		cluster, err = uncli.GetCurrentCluster()
		if err != nil {
			if errors.Is(err, cli.ErrNotFound) {
				// Do not create a default cluster if there are already clusters but the current cluster is not set.
				clusters, cErr := uncli.ListClusters()
				if cErr != nil {
					return fmt.Errorf("list clusters: %w", cErr)
				}
				if len(clusters) > 0 {
					return errors.New("the current cluster is not set in the Uncloud config. " +
						"Please specify a cluster with the --cluster flag or set current_cluster in the config")
				}

				cluster, err = uncli.CreateDefaultCluster()
				if err != nil {
					return fmt.Errorf("create default cluster: %w", err)
				}
				fmt.Printf("Created %q cluster\n", cluster.Name)
			} else {
				return fmt.Errorf("get current cluster: %w", err)
			}
		}
	} else {
		cluster, err = uncli.GetCluster(opts.cluster)
		if err != nil {
			return fmt.Errorf("get cluster %q: %w", opts.cluster, err)
		}
	}

	name, err := cluster.AddMachine(ctx, opts.name, opts.user, host, opts.port, opts.sshKey)
	if err != nil {
		return fmt.Errorf("add machine to cluster %q: %w", cluster.Name, err)
	}
	fmt.Printf("Machine %q added to cluster %q\n", name, cluster.Name)
	return nil
}
