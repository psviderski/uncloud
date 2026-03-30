package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func NewListCommand(groupID string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli)
		},
		GroupID: groupID,
	}
	return cmd
}

func list(ctx context.Context, uncli *cli.CLI) error {
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

	// Print the list of services in a table format.
	t := tui.NewTable()

	// Include the ID column if there are duplicate service names to differentiate them.
	headers := []string{"NAME", "MODE", "REPLICAS", "IMAGE", "ENDPOINTS"}
	if haveDuplicateNames {
		headers = append([]string{"ID"}, headers...)
	}
	t.Headers(headers...)

	for _, s := range services {
		images := s.Images()
		for i, img := range images {
			images[i] = tui.FormatImage(img, tui.NoStyle)
		}
		formattedImages := strings.Join(images, tui.Faint.Render(", "))
		endpoints := strings.Join(s.Endpoints(), tui.Faint.Render(", "))

		// If no endpoints from ports, check if the service uses custom Caddy config.
		if endpoints == "" {
			for _, ctr := range s.Containers {
				if ctr.Container.ServiceSpec.CaddyConfig() != "" {
					endpoints = "(custom Caddy config)"
				}
			}
		}

		row := []string{s.Name, s.Mode, fmt.Sprintf("%d", len(s.Containers)), formattedImages, endpoints}
		if haveDuplicateNames {
			row = append([]string{s.ID}, row...)
		}
		t.Row(row...)
	}

	fmt.Println(t)
	return nil
}
