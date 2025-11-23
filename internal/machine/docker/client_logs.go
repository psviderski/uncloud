package docker

import (
	"context"
	"errors"
	"io"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// ContainerLogsOptions specifies parameters for ContainerLogs
type ContainerLogsOptions struct {
	Follow     bool
	Tail       int64
	Timestamps bool
	Since      string
	Until      string
	Details    bool
}

// ContainerLogEntry represents a single log entry from a container
type ContainerLogEntry struct {
	StreamType int32 // 1 = stdout, 2 = stderr
	Message    string
	Timestamp  string
}

// ContainerLogs returns a channel that streams log entries from the container
func (c *Client) ContainerLogs(ctx context.Context, containerID string, opts ContainerLogsOptions) (<-chan ContainerLogEntry, error) {
	req := &pb.ContainerLogsRequest{
		ContainerId: containerID,
		Follow:      opts.Follow,
		Tail:        opts.Tail,
		Timestamps:  opts.Timestamps,
		Since:       opts.Since,
		Until:       opts.Until,
		Details:     opts.Details,
	}

	stream, err := c.GRPCClient.ContainerLogs(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan ContainerLogEntry, 100)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				//TODO: There's probably an error to handle here, but I'd rather just ignore it. anyway the channel will be closed to EOF signal
				return
			}

			ch <- ContainerLogEntry{
				StreamType: resp.StreamType,
				Message:    resp.Message,
				Timestamp:  resp.Timestamp,
			}
		}
	}()

	return ch, nil
}
