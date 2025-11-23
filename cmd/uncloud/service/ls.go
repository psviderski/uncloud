package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	format string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")
	return cmd
}

type serviceItem struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Mode      string   `json:"mode"`
	Replicas  int      `json:"replicas"`
	Images    []string `json:"images"`
	Endpoints []string `json:"endpoints"`
	// CustomCaddy indicates if the service uses a custom Caddy config.
	// This is used for display purposes when no endpoints are detected.
	CustomCaddy bool `json:"customCaddy"`
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	services, err := client.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	// Sort services by name.
	haveDuplicateNames := false
	slices.SortFunc(services, func(a, b api.Service) int {
		if a.Name == b.Name {
			haveDuplicateNames = true
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Name, b.Name)
	})

	var items []serviceItem
	for _, s := range services {
		endpoints := s.Endpoints()
		customCaddy := false

		// If no endpoints from ports, check if the service uses custom Caddy config.
		if len(endpoints) == 0 {
			for _, ctr := range s.Containers {
				if ctr.Container.ServiceSpec.CaddyConfig() != "" {
					customCaddy = true
					break
				}
			}
		}

		items = append(items, serviceItem{
			ID:          s.ID,
			Name:        s.Name,
			Mode:        string(s.Mode),
			Replicas:    len(s.Containers),
			Images:      s.Images(),
			Endpoints:   endpoints,
			CustomCaddy: customCaddy,
		})
	}

	columns := []output.Column[serviceItem]{}

	// Include the ID column if there are duplicate service names to differentiate them.
	if haveDuplicateNames {
		columns = append(columns, output.Column[serviceItem]{
			Header: "ID",
			Field:  "ID",
		})
	}

	columns = append(columns,
		output.Column[serviceItem]{
			Header: "NAME",
			Field:  "Name",
		},
		output.Column[serviceItem]{
			Header: "MODE",
			Field:  "Mode",
		},
		output.Column[serviceItem]{
			Header: "REPLICAS",
			Field:  "Replicas",
		},
		output.Column[serviceItem]{
			Header: "IMAGE",
			Field:  "Images",
		},
		output.Column[serviceItem]{
			Header: "ENDPOINTS",
			Accessor: func(item serviceItem) string {
				if len(item.Endpoints) > 0 {
					return strings.Join(item.Endpoints, ", ")
				}
				if item.CustomCaddy {
					return "(custom Caddy config)"
				}
				return ""
			},
		},
	)

	return output.Print(items, columns, opts.format)
}
