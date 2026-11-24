package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/api/pb"
	cliprogress "github.com/psviderski/uncloud/internal/cli/progress"
	"github.com/psviderski/uncloud/internal/grpcversion"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConnectHub connects the cluster to Uncloud Hub at the endpoint with a single-use connection token issued in Hub.
// Every machine then opens a tunnel to Hub, see WaitHubTunnel.
func (cli *Client) ConnectHub(ctx context.Context, endpoint, token string) (*pb.HubConnection, error) {
	return cli.ClusterClient.ConnectHub(ctx, &pb.ConnectHubRequest{Endpoint: endpoint, Token: token})
}

// DisconnectHub disconnects the cluster from Hub and returns the removed connection. Every machine then closes its
// tunnel to Hub. It returns api.ErrNotFound if the cluster isn't connected.
func (cli *Client) DisconnectHub(ctx context.Context) (*pb.HubConnection, error) {
	conn, err := cli.ClusterClient.DisconnectHub(ctx, nil)
	if status.Code(err) == codes.NotFound {
		return nil, api.ErrNotFound
	}
	return conn, err
}

// InspectHubConnection returns the connection of the cluster to Hub or api.ErrNotFound if the cluster isn't
// connected.
func (cli *Client) InspectHubConnection(ctx context.Context) (*pb.HubConnection, error) {
	conn, err := cli.ClusterClient.InspectHubConnection(ctx, nil)
	if status.Code(err) == codes.NotFound {
		return nil, api.ErrNotFound
	}
	return conn, err
}

// WaitHubTunnelOptions configures WaitHubTunnel.
type WaitHubTunnelOptions struct {
	// ClusterID is the ID of the cluster in Hub that the tunnel is for.
	ClusterID string
	// Close waits for the tunnel to close instead of opening.
	Close bool
}

// WaitHubTunnel waits for the tunnel from the machine to Hub to open or close and reports its progress to the progress
// writer from ctx. It waits until ctx is done and returns an error if the tunnel failed or didn't open or close
// in time.
func (cli *Client) WaitHubTunnel(ctx context.Context, machineNameOrID string, opts WaitHubTunnelOptions) error {
	start := time.Now()
	pw := progress.ContextWriter(ctx)
	eventID := cliprogress.HubTunnelEventID(machineNameOrID)
	pw.Event(progress.Waiting(eventID))

	// Stop watching the tunnel when the wait is done.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	tunnels, errs := cli.watchHubTunnel(ctx, machineNameOrID)

	// Update the retry countdown every second while the machine waits to retry.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var (
		last    *pb.HubTunnel
		lastErr error
	)
	for {
		select {
		case t := <-tunnels:
			last, lastErr = t, nil
			if done, err := reportHubTunnel(pw, eventID, t, opts); done {
				return err
			}
		case err := <-errs:
			if status.Code(err) == codes.Unimplemented {
				err = fmt.Errorf("machine daemon doesn't support Hub, upgrade it to the latest version: %s: %w",
					grpcversion.ReleaseURL, err)
				pw.Event(progress.ErrorMessageEvent(eventID, err.Error()))
				return err
			}
			// The stream ends if the machine restarts, so watch it again until ctx is done.
			lastErr = err
			pw.Event(progress.NewEvent(eventID, progress.Working,
				fmt.Sprintf("Reconnecting (lost connection to machine: %v)", err)))
		case <-ticker.C:
			if last != nil && last.State == pb.HubTunnel_RETRYING {
				reportHubTunnel(pw, eventID, last, opts)
			}
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}

			reason := "no response from machine"
			switch {
			case lastErr != nil:
				reason = lastErr.Error()
			case last != nil && last.LastError != "":
				reason = last.LastError
			case last != nil:
				reason = "tunnel is " + strings.ToLower(last.State.String())
			}

			action := "not connected"
			if opts.Close {
				action = "not closed"
			}
			msg := fmt.Sprintf("%s after %s: %s", action, time.Since(start).Round(time.Second), reason)
			pw.Event(progress.ErrorMessageEvent(eventID, msg))
			return fmt.Errorf("%s: %w", msg, ctx.Err())
		}
	}
}

// watchHubTunnel streams the state of the tunnel from the machine to Hub until ctx is cancelled. It reopens the stream
// when it fails and sends the error to the errors channel, except Unimplemented after which it stops.
func (cli *Client) watchHubTunnel(ctx context.Context, machineNameOrID string) (<-chan *pb.HubTunnel, <-chan error) {
	tunnels := make(chan *pb.HubTunnel)
	errs := make(chan error)

	go func() {
		send := func(err error) bool {
			select {
			case errs <- err:
				return status.Code(err) != codes.Unimplemented
			case <-ctx.Done():
				return false
			}
		}

		for ctx.Err() == nil {
			stream, err := cli.MachineClient.InspectHubTunnel(ProxySingleMachineContext(ctx, machineNameOrID),
				&pb.InspectHubTunnelRequest{Watch: true})
			for err == nil {
				var t *pb.HubTunnel
				if t, err = stream.Recv(); err == nil {
					select {
					case tunnels <- t:
					case <-ctx.Done():
						return
					}
				}
			}
			if ctx.Err() != nil || !send(err) {
				return
			}

			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()

	return tunnels, errs
}

// reportHubTunnel reports the state of the tunnel and whether the wait is done. It returns an error if the tunnel
// failed.
func reportHubTunnel(
	pw progress.Writer, eventID string, t *pb.HubTunnel, opts WaitHubTunnelOptions,
) (bool, error) {
	if opts.Close {
		if t.ClusterId != opts.ClusterID || t.State == pb.HubTunnel_INACTIVE {
			pw.Event(progress.NewEvent(eventID, progress.Done, "Closed"))
			return true, nil
		}
		pw.Event(progress.NewEvent(eventID, progress.Working, "Closing"))
		return false, nil
	}

	if t.ClusterId != opts.ClusterID {
		pw.Event(progress.NewEvent(eventID, progress.Working, "Waiting cluster connection to sync"))
		return false, nil
	}

	switch t.State {
	case pb.HubTunnel_CONNECTED:
		pw.Event(progress.NewEvent(eventID, progress.Done, "Connected"))
		return true, nil
	case pb.HubTunnel_RETRYING:
		retryIn := time.Until(t.RetryAt.AsTime()).Round(time.Second)
		text := "Retrying"
		if retryIn > 0 {
			text = fmt.Sprintf("Retrying in %s", retryIn)
		}
		if t.LastError != "" {
			text += " (" + t.LastError + ")"
		}
		pw.Event(progress.NewEvent(eventID, progress.Working, text))
	case pb.HubTunnel_FAILED:
		pw.Event(progress.ErrorMessageEvent(eventID, "Failed ("+t.LastError+")"))
		return true, errors.New(t.LastError)
	default:
		pw.Event(progress.NewEvent(eventID, progress.Working, "Connecting"))
	}
	return false, nil
}
