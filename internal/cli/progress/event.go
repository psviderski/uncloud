package progress

import (
	"context"

	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli/tui"
)

type eventIDKey struct{}

// WithEventID returns a context that overrides the default event ID used by client methods.
func WithEventID(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, eventIDKey{}, eventID)
}

// ContainerEventID returns a progress event ID for operations on existing containers using the canonical
// service_name/short_id format. Allows to override it using WithEventID.
func ContainerEventID(ctx context.Context, serviceName, containerID, machineName string) string {
	if id, ok := ctx.Value(eventIDKey{}).(string); ok && id != "" {
		return id
	}
	return tui.Faint.Render("Container ") +
		serviceName + tui.Faint.Render("/") + stringid.TruncateID(containerID) +
		tui.Faint.Render(" on ") + machineName
}

// NewContainerEventID returns a progress event ID for new container creation where the Docker container ID
// is not yet known. Allows to override it using WithEventID.
func NewContainerEventID(ctx context.Context, containerName, machineName string) string {
	if id, ok := ctx.Value(eventIDKey{}).(string); ok && id != "" {
		return id
	}
	return tui.Faint.Render("Container ") + containerName +
		tui.Faint.Render(" on ") + machineName
}

// PreDeployHookEventID returns a progress event ID for pre-deploy hook operations.
func PreDeployHookEventID(serviceName, machineName string) string {
	return tui.Faint.Render("Pre-deploy hook ") + serviceName +
		tui.Faint.Render(" on ") + machineName
}

// OldPreDeployHookEventID returns a progress event ID for old pre-deploy hook container cleanup.
func OldPreDeployHookEventID(serviceName, containerID, machineName string) string {
	return tui.Faint.Render("Old pre-deploy hook ") +
		serviceName + tui.Faint.Render("/") + stringid.TruncateID(containerID) +
		tui.Faint.Render(" on ") + machineName
}

// ImageEventID returns a progress event ID for image pull operations.
func ImageEventID(image, machineName string) string {
	return tui.Faint.Render("Image ") + image +
		tui.Faint.Render(" on ") + machineName
}

// VolumeEventID returns a progress event ID for volume operations.
func VolumeEventID(volumeName, machineName string) string {
	return tui.Faint.Render("Volume ") + volumeName +
		tui.Faint.Render(" on ") + machineName
}

// MachineEventID returns a progress event ID for machine operations.
func MachineEventID(machineName, publicIP string) string {
	return tui.Faint.Render("Machine ") + machineName +
		tui.Faint.Render(" (") + publicIP + tui.Faint.Render(")")
}
