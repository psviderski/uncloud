package cluster

import (
	"fmt"
	"strings"

	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List clusters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := cmd.Context().Value("provisioner").(*ucind.Provisioner)
			clusters, err := p.ListClusters(cmd.Context())
			if err != nil {
				return fmt.Errorf("list clusters: %w", err)
			}
			t := tui.NewTable()
			t.Headers("NAME", "MACHINES")
			for _, cluster := range clusters {
				machines := []string{}
				for _, m := range cluster.Machines {
					machines = append(machines, m.Name)
				}
				t.Row(
					cluster.Name,
					strings.Join(machines, tui.Faint.Render(", ")),
				)
			}
			fmt.Println(t)
			return nil
		},
	}
	return cmd
}
