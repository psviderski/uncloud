package image

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type pullOptions struct {
	machines []string
	all      bool
}

func NewPullCommand() *cobra.Command {
	opts := pullOptions{}

	cmd := &cobra.Command{
		Use:   "pull [NAME]",
		Short: "Pull an image from a remote registry to the cluster.",
		Long:  "Pull an image from a remote registry to the cluster. By default, to all machines.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return pull(cmd.Context(), uncli, args[0], opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter machines to pull image on. Can be specified multiple times or as a comma-separated list. "+
			"(default is all machines)")
	cmd.Flags().BoolVarP(&opts.all, "all-tags", "a", false, "Download all tagged images in the repository")

	return cmd
}

func pull(ctx context.Context, uncli *cli.CLI, img string, opts pullOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	pullOpts := image.PullOptions{
		All: opts.all,
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		ch, err := clusterClient.PullImage(ctx, img, pullOpts, machines)
		if err != nil {
			return fmt.Errorf("pull image: %w", err)
		}

		pw := progress.ContextWriter(ctx)
		started := false
		eventID := fmt.Sprintf("Pulling %s", img)

		for msg := range ch {
			if msg.Err != nil {
				return fmt.Errorf("pull image: %w", msg.Err)
			}
			if !started {
				pw.Event(progress.Event{
					ID:         eventID,
					Status:     progress.Working,
					StatusText: "Pulling...",
				})
				started = true
			}

			if msg.Message.ID != "" {
				e := toProgressEvent(msg.Message)
				if e != nil {
					e.ParentID = eventID
					pw.Event(*e)
				}
			} else if msg.Message.Status != "" {
				pw.Event(progress.Event{
					ID:       eventID,
					ParentID: eventID,
					Status:   progress.Working,
					Text:     msg.Message.Status,
				})
			}
		}

		pw.Event(progress.Event{
			ID:         eventID,
			Status:     progress.Done,
			StatusText: "Pulled",
		})

		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Pulling image %s on cluster", img))
}

func toProgressEvent(jm jsonmessage.JSONMessage) *progress.Event {
	if jm.ID == "" || jm.Progress == nil {
		return nil
	}

	status := progress.Working
	percent := 0

	if jm.Progress.Total > 0 {
		percent = int(jm.Progress.Current * 100 / jm.Progress.Total)
		if percent > 100 {
			percent = 100
		}
	}

	switch jm.Status {
	case "Pull complete", "Already exists", "Download complete":
		status = progress.Done
		percent = 100
	}

	return &progress.Event{
		ID:         jm.ID,
		Current:    jm.Progress.Current,
		Total:      jm.Progress.Total,
		Percent:    percent,
		Text:       jm.Status,
		Status:     status,
		StatusText: jm.Progress.String(),
	}
}
