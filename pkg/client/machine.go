package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/moby/term"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (cli *Client) ExecMachine(ctx context.Context, machineNameOrID string, opts api.MachineExecOptions) (int, error) {
	if len(opts.Command) == 0 {
		return -1, fmt.Errorf("command is required")
	}

	machine, err := cli.InspectMachine(ctx, machineNameOrID)
	if err != nil {
		return -1, fmt.Errorf("inspect machine '%s': %w", machineNameOrID, err)
	}

	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	ctx = cli.ProxySingleMachineContext(ctx, machine.Machine.Id)
	stream, err := cli.MachineClient.ExecCommand(ctx)
	if err != nil {
		return -1, fmt.Errorf("create exec command stream to machine '%s': %w", machine.Machine.Name, err)
	}

	var sendMu sync.Mutex
	send := func(req *pb.ExecCommandRequest) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(req)
	}

	if err = send(&pb.ExecCommandRequest{
		Payload: &pb.ExecCommandRequest_Config{
			Config: &pb.ExecCommandConfig{
				Command:     opts.Command,
				AttachStdin: opts.AttachStdin,
				Tty:         opts.Tty,
			},
		},
	}); err != nil {
		return -1, fmt.Errorf("send exec command config: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.AttachStdin && opts.Tty {
		restoreTerminal, err := setupMachineExecTerminal(ctx, send)
		if err != nil {
			return -1, fmt.Errorf("setup terminal: %w", err)
		}
		defer restoreTerminal()
	}

	exitCode := 1
	errGroup, ctx := errgroup.WithContext(ctx)

	if opts.AttachStdin {
		errGroup.Go(func() error {
			return handleMachineExecInput(ctx, stream, send, stdin)
		})
	} else {
		if err = stream.CloseSend(); err != nil {
			return -1, fmt.Errorf("close exec command input stream: %w", err)
		}
	}

	errGroup.Go(func() error {
		defer cancel()
		code, err := handleMachineExecOutput(ctx, stream, stdout, stderr)
		if err != nil {
			return err
		}
		exitCode = code
		return nil
	})

	if err = errGroup.Wait(); err != nil {
		return exitCode, err
	}

	return exitCode, nil
}

func setupMachineExecTerminal(ctx context.Context, send func(*pb.ExecCommandRequest) error) (func(), error) {
	inFd, isTerminal := term.GetFdInfo(os.Stdin)
	if !isTerminal {
		return nil, fmt.Errorf("stdin is not a terminal")
	}

	oldState, err := term.SetRawTerminal(inFd)
	if err != nil {
		return nil, fmt.Errorf("set raw terminal: %w", err)
	}
	restore := func() {
		_ = term.RestoreTerminal(inFd, oldState)
	}

	if err = handleMachineExecTerminalResize(ctx, inFd, send); err != nil {
		restore()
		return nil, err
	}

	return restore, nil
}

func handleMachineExecTerminalResize(ctx context.Context, inFd uintptr, send func(*pb.ExecCommandRequest) error) error {
	sendResize := func() {
		size, err := term.GetWinsize(inFd)
		if err != nil {
			return
		}
		_ = send(&pb.ExecCommandRequest{
			Payload: &pb.ExecCommandRequest_Resize{
				Resize: &pb.ExecCommandResizeEvent{
					Height: uint32(size.Height),
					Width:  uint32(size.Width),
				},
			},
		})
	}

	sendResize()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGWINCH)
	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				sendResize()
			}
		}
	}()

	return nil
}

func handleMachineExecInput(
	ctx context.Context,
	stream pb.Machine_ExecCommandClient,
	send func(*pb.ExecCommandRequest) error,
	stdin io.Reader,
) error {
	defer stream.CloseSend()

	stdinCh := make(chan []byte)
	stdinErrCh := make(chan error, 1)

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case stdinCh <- data:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				stdinErrCh <- err
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case data := <-stdinCh:
			if err := send(&pb.ExecCommandRequest{
				Payload: &pb.ExecCommandRequest_Stdin{Stdin: data},
			}); err != nil {
				return fmt.Errorf("send stdin: %w", err)
			}
		case err := <-stdinErrCh:
			if err != io.EOF {
				return fmt.Errorf("read stdin: %w", err)
			}
			return nil
		}
	}
}

func handleMachineExecOutput(
	_ context.Context,
	stream pb.Machine_ExecCommandClient,
	stdout, stderr io.Writer,
) (int, error) {
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return -1, fmt.Errorf("exec command stream closed without exit code")
		}
		if err != nil {
			return -1, fmt.Errorf("receive exec command response: %w", err)
		}

		switch payload := resp.Payload.(type) {
		case *pb.ExecCommandResponse_Stdout:
			if _, err := stdout.Write(payload.Stdout); err != nil {
				return -1, fmt.Errorf("write stdout: %w", err)
			}
		case *pb.ExecCommandResponse_Stderr:
			if _, err := stderr.Write(payload.Stderr); err != nil {
				return -1, fmt.Errorf("write stderr: %w", err)
			}
		case *pb.ExecCommandResponse_ExitCode:
			return int(payload.ExitCode), nil
		}
	}
}

func (cli *Client) InspectMachine(ctx context.Context, nameOrID string) (*pb.MachineMember, error) {
	// TODO: refactor to use MachineClient.InspectMachine.
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, m := range machines {
		if m.Machine.Id == nameOrID || m.Machine.Name == nameOrID {
			return m, nil
		}
	}

	return nil, api.ErrNotFound
}

// ListMachines returns a list of all machines registered in the cluster that match the filter.
func (cli *Client) ListMachines(ctx context.Context, filter *api.MachineFilter) (api.MachineMembersList, error) {
	resp, err := cli.ClusterClient.ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	machines := api.MachineMembersList(resp.Machines)

	if filter == nil {
		return machines, nil
	}

	// Apply the filter.
	if len(filter.NamesOrIDs) > 0 {
		var matched api.MachineMembersList
		var notFound []string

		for _, nameOrID := range filter.NamesOrIDs {
			if m := machines.FindByNameOrID(nameOrID); m != nil {
				matched = append(matched, m)
			} else {
				notFound = append(notFound, nameOrID)
			}
		}
		machines = matched

		if len(notFound) > 0 {
			return nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
		}
	}

	if filter.Available {
		var available api.MachineMembersList
		for _, m := range machines {
			if m.State != pb.MachineMember_DOWN {
				available = append(available, m)
			}
		}
		machines = available
	}

	return machines, nil
}

// UpdateMachine updates machine configuration in the cluster.
func (cli *Client) UpdateMachine(ctx context.Context, req *pb.UpdateMachineRequest) (*pb.MachineInfo, error) {
	resp, err := cli.ClusterClient.UpdateMachine(ctx, req)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return nil, api.ErrNotFound
		}
		return nil, err
	}
	return resp.Machine, nil
}

// RenameMachine renames an existing machine in the cluster.
func (cli *Client) RenameMachine(ctx context.Context, nameOrID, newName string) (*pb.MachineInfo, error) {
	// First, resolve the machine to get its ID
	machine, err := cli.InspectMachine(ctx, nameOrID)
	if err != nil {
		return nil, err
	}

	// Update the machine with the new name
	req := &pb.UpdateMachineRequest{
		MachineId: machine.Machine.Id,
		Name:      &newName,
	}

	return cli.UpdateMachine(ctx, req)
}

// WaitMachineReady waits for the machine API on the connected machine to respond.
func (cli *Client) WaitMachineReady(ctx context.Context, timeout time.Duration) error {
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	inspect := func() error {
		if _, err := cli.Inspect(ctx, &emptypb.Empty{}); err != nil {
			return fmt.Errorf("inspect machine: %w", err)
		}
		return nil
	}
	return backoff.Retry(inspect, boff)
}

// WaitClusterReady waits for the connected machine to be ready to serve cluster requests.
func (cli *Client) WaitClusterReady(ctx context.Context, timeout time.Duration) error {
	// Backoff is not really needed here as the default service config for the gRPC client is already
	// doing retries with backoff for Unavailable errors. However, it's still convenient to use backoff
	// to control the overall timeout for the operation.
	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(100*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(timeout),
	), ctx)

	listMachines := func() error {
		_, err := cli.ListMachines(ctx, nil)
		if err != nil {
			if s, ok := status.FromError(err); ok &&
				// TODO: remove FailedPrecondition after releading 0.17.
				(s.Code() == codes.Unavailable || s.Code() == codes.FailedPrecondition) {
				// Machine is not ready yet, retry.
				return err
			}
			// Other non-Unavailable errors should not be retried.
			return backoff.Permanent(err)
		}
		return nil
	}
	return backoff.Retry(listMachines, boff)
}
