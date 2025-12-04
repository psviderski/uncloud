package service

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	service string
	format  string
}

func NewInspectCommand() *cobra.Command {
	opts := inspectOptions{}
	cmd := &cobra.Command{
		Use:   "inspect SERVICE",
		Short: "Display detailed information on a service.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.service = args[0]
			return inspect(cmd.Context(), uncli, opts)
		},
	}
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")
	return cmd
}

type containerItem struct {
	ID        string `json:"id"`
	Image     string `json:"image"`
	Created   int64  `json:"created"`
	Status    string `json:"status"`
	StartedAt int64  `json:"startedAt,omitempty"`
	Machine   string `json:"machine"`
}

type serviceInspectJSON struct {
	ID         string          `json:"serviceID"`
	Name       string          `json:"name"`
	Mode       string          `json:"mode"`
	Containers []containerItem `json:"containers"`
}

func inspect(ctx context.Context, uncli *cli.CLI, opts inspectOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	svc, err := client.InspectService(ctx, opts.service)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machinesNamesByID := make(map[string]string)
	for _, m := range machines {
		if machineMember := m.Machine; machineMember != nil {
			machinesNamesByID[machineMember.Id] = machineMember.Name
		}
	}

	var containers []containerItem
	for _, ctr := range svc.Containers {
		createdAt, err := time.Parse(time.RFC3339Nano, ctr.Container.Created)
		if err != nil {
			return fmt.Errorf("parse created time: %w", err)
		}

		machine := machinesNamesByID[ctr.MachineID]
		if machine == "" {
			machine = ctr.MachineID
		}
		state, err := ctr.Container.HumanState()
		if err != nil {
			return fmt.Errorf("get human state: %w", err)
		}

		var startedAt int64
		if ctr.Container.State != nil && ctr.Container.State.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, ctr.Container.State.StartedAt); err == nil {
				startedAt = t.Unix()
			}
		}

		containers = append(containers, containerItem{
			ID:        stringid.TruncateID(ctr.Container.ID),
			Image:     ctr.Container.Config.Image,
			Created:   createdAt.Unix(),
			Status:    state,
			StartedAt: startedAt,
			Machine:   machine,
		})
	}

	if opts.format == "json" {
		data := serviceInspectJSON{
			ID:         svc.ID,
			Name:       svc.Name,
			Mode:       svc.Mode,
			Containers: containers,
		}
		return output.Print[any](data, nil, "json")
	}

	// Table Output
	fmt.Printf("Service ID: %s\n", svc.ID)
	fmt.Printf("Name:       %s\n", svc.Name)
	fmt.Printf("Mode:       %s\n", svc.Mode)
	fmt.Println()

	columns := []output.Column[containerItem]{
		{Header: "CONTAINER ID", Field: "ID"},
		{Header: "IMAGE", Field: "Image"},
		{
			Header: "CREATED",
			Accessor: func(i containerItem) string {
				createdAt := time.Unix(i.Created, 0)
				return units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
			},
		},
		{Header: "STATUS", Field: "Status"},
		{Header: "MACHINE", Field: "Machine"},
	}

	return output.Print(containers, columns, "table")
}
