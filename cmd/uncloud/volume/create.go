package volume

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type createOptions struct {
	driver     string
	driverOpts []string
	labels     []string
	machine    string
}

func NewCreateCommand() *cobra.Command {
	opts := createOptions{}

	cmd := &cobra.Command{
		Use:   "create VOLUME_NAME",
		Short: "Create a volume on a specific machine.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.driver = strings.TrimSpace(opts.driver)

			volumeName := args[0]
			if volumeName == "" {
				return fmt.Errorf("volume name is required")
			}

			return create(cmd.Context(), uncli, volumeName, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.driver, "driver", "d", "local",
		"Volume driver to use.")
	cmd.Flags().StringSliceVarP(&opts.driverOpts, "opt", "o", nil,
		"Driver specific options in the form of 'key=value' pairs. Can be specified multiple times.")
	cmd.Flags().StringSliceVarP(&opts.labels, "label", "l", nil,
		"Labels to assign to the volume in the form of 'key=value' pairs. Can be specified multiple times.")
	cmd.Flags().StringVarP(&opts.machine, "machine", "m", "",
		"Name or ID of the machine to create the volume on.")

	return cmd
}

func create(ctx context.Context, uncli *cli.CLI, name string, opts createOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Parse driver options.
	driverOpts := make(map[string]string)
	for _, opt := range opts.driverOpts {
		k, v, ok := strings.Cut(opt, "=")
		if !ok {
			return fmt.Errorf("invalid driver option format: '%s' (expected key=value)", opt)
		}
		driverOpts[k] = v
	}

	// Parse labels.
	labels := make(map[string]string)
	for _, label := range opts.labels {
		k, v, ok := strings.Cut(label, "=")
		if !ok {
			return fmt.Errorf("invalid label format: '%s' (expected key=value)", label)
		}
		labels[k] = v
	}

	// List machines and filter by the specified machine name or ID.
	// If no machine is specified, prompt the user to select one.
	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	var selectedMachine *pb.MachineInfo

	if opts.machine == "" {
		if len(machines) == 1 {
			selectedMachine = machines[0].Machine
		} else {
			if selectedMachine, err = promptSelectMachine(ctx, machines); err != nil {
				return fmt.Errorf("select machine: %w", err)
			}
		}
	} else {
		m := machines.FindByNameOrID(opts.machine)
		if m == nil {
			return fmt.Errorf("machine '%s' not found", opts.machine)
		}
		selectedMachine = m.Machine
	}

	createOpts := volume.CreateOptions{
		Name:       name,
		Driver:     opts.driver,
		DriverOpts: driverOpts,
		Labels:     labels,
	}
	vol, err := client.CreateVolume(ctx, selectedMachine.Id, createOpts)
	if err != nil {
		return fmt.Errorf("create volume '%s' on machine '%s': %w", name, selectedMachine.Name, err)
	}

	fmt.Printf("Volume '%s' created on machine '%s'.\n", vol.Volume.Name, vol.MachineName)
	return nil
}

func promptSelectMachine(ctx context.Context, machines api.MachineMembersList) (*pb.MachineInfo, error) {
	options := make([]huh.Option[*pb.MachineInfo], len(machines))
	for i, m := range machines {
		options[i] = huh.NewOption(m.Machine.Name, m.Machine)
	}
	slices.SortFunc(options, func(a, b huh.Option[*pb.MachineInfo]) int {
		return strings.Compare(a.Key, b.Key)
	})

	var selected *pb.MachineInfo
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[*pb.MachineInfo]().
				Title("Select a machine to create the volume on (or specify with --machine flag)").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.RunWithContext(ctx); err != nil {
		return nil, err
	}

	return selected, nil
}
