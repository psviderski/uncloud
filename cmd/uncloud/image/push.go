package image

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type pushOptions struct {
	machines []string
}

func NewPushCommand() *cobra.Command {
	opts := pushOptions{}

	cmd := &cobra.Command{
		Use:   "push IMAGE",
		Short: "Upload a local Docker image to the cluster.",
		Long: `Upload a local Docker image to the cluster transferring only the missing layers.
The image is uploaded to the machine which CLI is connected to (default) or the specified machine(s).`,
		Example: `  # Push image to the currently connected machine.
  uc image push myapp:latest

  # Push image to specific machine.
  uc image push myapp:latest -m machine1

  # Push image to multiple machines.
  uc image push myapp:latest -m machine1,machine2,machine3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			image := args[0]

			machines := cli.ExpandCommaSeparatedValues(opts.machines)

			// TODO: Implement image push logic
			fmt.Printf("Would push image %q to machines: %v\n", image, machines)
			return fmt.Errorf("image push not yet implemented")
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names to push the image to. Can be specified multiple times or as a comma-separated "+
			"list of machine names. (default is connected machine)")

	return cmd
}
