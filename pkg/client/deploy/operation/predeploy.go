package operation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/containerd/errdefs"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	cliprogress "github.com/psviderski/uncloud/internal/cli/progress"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
)

// PreDeployHookError indicates that a pre-deploy hook container exited with a non-zero code or timed out.
type PreDeployHookError struct {
	error
	ServiceName string
	ContainerID string
	MachineName string
}

func (e *PreDeployHookError) Unwrap() error {
	return e.error
}

// DefaultPreDeployTimeout is the maximum duration to wait for a pre-deploy hook container to complete.
const DefaultPreDeployTimeout = 5 * time.Minute

// StopPreDeployOperation stops a running pre-deploy hook container from a previous deployment.
type StopPreDeployOperation struct {
	MachineID string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName string
	Container   api.ServiceContainer
}

func (o *StopPreDeployOperation) Execute(ctx context.Context, cli Client) error {
	ctx = cliprogress.WithEventID(ctx,
		cliprogress.OldPreDeployHookEventID(o.Container.ServiceName(), o.Container.ID, o.MachineName))
	if err := cli.StopContainer(ctx, o.Container.ServiceID(), o.Container.ID, container.StopOptions{}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("stop pre-deploy hook container '%s': %w", o.Container.ID, err)
		}
	}
	return nil
}

func (o *StopPreDeployOperation) Format() string {
	displayName := o.Container.ServiceSpec.Name + tui.Faint.Render("/") + o.Container.ShortID()
	status, _ := o.Container.HumanState()

	return tui.BoldRed.Render("⏹") + "   " +
		tui.Faint.Render("stop pre-deploy hook") + " " +
		displayName + " " +
		tui.Faint.Render("("+status+")") + " " +
		tui.Faint.Render("on") + " " +
		o.MachineName
}

func (o *StopPreDeployOperation) String() string {
	return fmt.Sprintf("StopPreDeployOperation[machine_id=%s container_id=%s]",
		o.MachineID, o.Container.ID)
}

// RunPreDeployOperation runs a one-shot pre-deploy hook container before service deployment.
type RunPreDeployOperation struct {
	ServiceID string
	Spec      api.ServiceSpec
	MachineID string
	// MachineName is used for formatting the operation as part of the deployment plan.
	MachineName string
	// OldContainerIDs are pre-deploy hook containers from previous deployments to remove.
	OldContainerIDs []string
}

func (o *RunPreDeployOperation) Execute(ctx context.Context, cli Client) error {
	// Remove old pre-deploy containers.
	for _, id := range o.OldContainerIDs {
		oldCtx := cliprogress.WithEventID(ctx, cliprogress.OldPreDeployHookEventID(o.Spec.Name, id, o.MachineName))
		_ = cli.StopContainer(oldCtx, o.ServiceID, id, container.StopOptions{})
		err := cli.RemoveContainer(oldCtx, o.ServiceID, id, container.RemoveOptions{RemoveVolumes: true})
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove old pre-deploy hook container '%s': %w", id, err)
		}
	}

	// Set the event ID override so all downstream client methods (create, start, stop) use the same
	// pre-deploy hook progress event instead of the generic container one.
	ctx = cliprogress.WithEventID(ctx, cliprogress.PreDeployHookEventID(o.Spec.Name, o.MachineName))

	resp, err := cli.CreatePreDeployHookContainer(ctx, o.ServiceID, o.Spec, o.MachineID)
	if err != nil {
		return fmt.Errorf("create pre-deploy hook container: %w", err)
	}
	if err = cli.StartContainer(ctx, o.ServiceID, resp.ID); err != nil {
		return fmt.Errorf("start pre-deploy hook container: %w", err)
	}

	timeout := DefaultPreDeployTimeout
	if o.Spec.PreDeploy.Timeout != nil {
		timeout = *o.Spec.PreDeploy.Timeout
	}

	return o.waitForExit(ctx, cli, resp.ID, timeout)
}

// waitForExit polls the container state until it exits or the timeout is reached.
func (o *RunPreDeployOperation) waitForExit(
	ctx context.Context, cli Client, containerID string, timeout time.Duration,
) error {
	pw := progress.ContextWriter(ctx)
	eventID := cliprogress.PreDeployHookEventID(o.Spec.Name, o.MachineName)
	pw.Event(progress.Waiting(eventID))

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var ctr *api.ServiceContainer
	for {
		select {
		case <-timeoutCtx.Done():
			// Stop the container on timeout or context cancellation.
			_ = cli.StopContainer(ctx, o.ServiceID, containerID, container.StopOptions{})

			ctrID := containerID
			if ctr != nil {
				ctrID = fmt.Sprintf("%s/%s", o.Spec.Name, ctr.ShortID())
			}
			if ctx.Err() != nil {
				// The parent context has been cancelled before the timeout.
				pw.Event(progress.NewEvent(eventID, progress.Error, "Cancelled"))
				return fmt.Errorf("pre-deploy hook container '%s': %w", ctrID, ctx.Err())
			}
			pw.Event(progress.NewEvent(eventID, progress.Error, fmt.Sprintf("Timeout (%s)", timeout)))
			return &PreDeployHookError{
				ServiceName: o.Spec.Name,
				ContainerID: containerID,
				MachineName: o.MachineName,
				error: fmt.Errorf("pre-deploy hook container '%s' timed out after %s. "+
					"It's stopped and available for inspection. View logs with 'uc logs %s'",
					ctrID, timeout, o.Spec.Name),
			}

		case <-ticker.C:
			mc, err := cli.InspectContainer(ctx, o.ServiceID, containerID)
			if err != nil {
				return fmt.Errorf("inspect pre-deploy hook container: %w", err)
			}
			ctr = &mc.Container

			if ctr.State.Running {
				if state, err := ctr.Container.HumanState(); err == nil {
					pw.Event(progress.NewEvent(eventID, progress.Working, fmt.Sprintf("Waiting (%s)", state)))
				}
				continue
			}

			// Hook container has exited successfully.
			if ctr.State.ExitCode == 0 {
				pw.Event(progress.Event{
					ID:     eventID,
					Status: progress.Done,
				})
				return nil
			}

			pw.Event(progress.ErrorEvent(eventID))
			ctrID := fmt.Sprintf("%s/%s", o.Spec.Name, ctr.ShortID())
			return &PreDeployHookError{
				ServiceName: o.Spec.Name,
				ContainerID: containerID,
				MachineName: o.MachineName,
				error: fmt.Errorf("pre-deploy hook container '%s' failed with exit code: %d. "+
					"It's stopped and available for inspection. View logs with 'uc logs %s'",
					ctrID, ctr.State.ExitCode, o.Spec.Name),
			}
		}
	}
}

func (o *RunPreDeployOperation) Format() string {
	cmd := strings.Join(o.Spec.PreDeploy.Command, " ")

	timeout := DefaultPreDeployTimeout
	if o.Spec.PreDeploy.Timeout != nil {
		timeout = *o.Spec.PreDeploy.Timeout
	}

	prefix := tui.BoldGreen.Render("▶") + "   " +
		tui.Faint.Render("run pre-deploy hook") + " " +
		o.Spec.Name + " ["
	suffix := "] " +
		tui.Faint.Render("on") + " " +
		o.MachineName + " " +
		tui.Yellow.Render(fmt.Sprintf("(timeout %s)", timeout))

	// Truncate the command to fit within the terminal width.
	termWidth := tui.TerminalWidth()
	if termWidth > 0 {
		buffer := 10
		maxCmdWidth := termWidth - lipgloss.Width(prefix) - lipgloss.Width(suffix) - buffer
		if maxCmdWidth <= 0 {
			maxCmdWidth = 10
		}
		cmd = ansi.Truncate(cmd, maxCmdWidth, "…")
	}

	return prefix + cmd + suffix
}

func (o *RunPreDeployOperation) String() string {
	return fmt.Sprintf("RunPreDeployOperation[machine_id=%s service_id=%s cmd=%v]",
		o.MachineID, o.ServiceID, o.Spec.PreDeploy.Command)
}
