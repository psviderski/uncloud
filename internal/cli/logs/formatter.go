package logs

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
)

// Formatter handles formatting and printing of log entries with dynamic column alignment.
type Formatter struct {
	machineNames []string
	serviceNames []string

	maxMachineWidth int
	maxServiceWidth int

	utc bool
}

func NewFormatter(machineNames, serviceNames []string, utc bool) *Formatter {
	slices.Sort(machineNames)
	slices.Sort(serviceNames)

	maxMachineWidth := 0
	for _, name := range machineNames {
		if len(name) > maxMachineWidth {
			maxMachineWidth = len(name)
		}
	}

	maxServiceWidth := 0
	for _, name := range serviceNames {
		if len(name) > maxServiceWidth {
			maxServiceWidth = len(name)
		}
	}

	return &Formatter{
		machineNames:    machineNames,
		serviceNames:    serviceNames,
		maxMachineWidth: maxMachineWidth,
		maxServiceWidth: maxServiceWidth,
		utc:             utc,
	}
}

// formatTimestamp formats timestamp using local timezone or UTC if configured.
func (f *Formatter) formatTimestamp(t time.Time) string {
	if f.utc {
		t = t.UTC()
	} else {
		t = t.In(time.Local)
	}
	dimStyle := lipgloss.NewStyle().Faint(true)

	return dimStyle.Render(t.Format(time.StampMilli))
}

func (f *Formatter) formatMachine(name string) string {
	style := lipgloss.NewStyle().Bold(true).PaddingRight(f.maxMachineWidth - len(name))

	if len(f.serviceNames) == 1 {
		// Machine name is coloured for single-service logs.
		i := slices.Index(f.machineNames, name)
		if i == -1 {
			f.machineNames = append(f.machineNames, name)
			i = len(f.machineNames) - 1
		}

		style = style.Foreground(palette[i%len(palette)])
	}

	return style.Render(name)
}

func (f *Formatter) formatService(serviceName, containerID, hook string) string {
	styleService := lipgloss.NewStyle().Bold(true)
	padding := f.maxServiceWidth - len(serviceName)

	if len(f.serviceNames) > 1 {
		// Service name is coloured for multi-service logs.
		i := slices.Index(f.serviceNames, serviceName)
		if i == -1 {
			f.serviceNames = append(f.serviceNames, serviceName)
			i = len(f.serviceNames) - 1
		}

		styleService = styleService.Foreground(palette[i%len(palette)])
	}

	// Journal logs are unit-scoped and have no container ID.
	if containerID == "" {
		return styleService.PaddingRight(padding).Render(serviceName)
	}

	out := styleService.Render(serviceName) + tui.Faint.PaddingRight(padding).Render("/"+containerID[:5])
	if hook != "" {
		out += tui.Faint.Render(" [" + hook + "]")
	}
	return out
}

// PrintEntry prints a single log entry with proper formatting.
func (f *Formatter) PrintEntry(entry api.ServiceLogEntry) {
	if entry.Err != nil {
		f.printError(entry)
		return
	}
	if entry.Stream != api.LogStreamStdout && entry.Stream != api.LogStreamStderr {
		return
	}

	var output strings.Builder

	// Timestamp
	output.WriteString(f.formatTimestamp(entry.Timestamp))
	output.WriteString(" ")

	// Machine name
	output.WriteString(f.formatMachine(entry.Metadata.MachineName))
	output.WriteString(" ")

	// Service/container_id or service name for a systemd service.
	output.WriteString(f.formatService(entry.Metadata.ServiceName, entry.Metadata.ContainerID, entry.Metadata.Hook))
	output.WriteString(" ")

	// Message
	output.Write(entry.Message)

	// Print to appropriate stream.
	if entry.Stream == api.LogStreamStderr {
		fmt.Fprint(os.Stderr, output.String())
	} else {
		fmt.Print(output.String())
	}
}

// printError prints an error entry (e.g., stalled stream warning).
func (f *Formatter) printError(entry api.ServiceLogEntry) {
	if entry.Metadata.ServiceName == "" {
		msg := fmt.Sprintf("ERROR: %v", entry.Err)
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightRed)
		fmt.Fprintln(os.Stderr, style.Render(msg))
		return
	}

	var msg string
	if entry.Metadata.ContainerID != "" {
		msg = fmt.Sprintf("WARNING: log stream from container '%s/%s' on machine '%s'",
			entry.Metadata.ServiceName,
			stringid.TruncateID(entry.Metadata.ContainerID),
			entry.Metadata.MachineName)
	} else {
		msg = fmt.Sprintf("WARNING: log stream from systemd service '%s' on machine '%s'",
			entry.Metadata.ServiceName,
			entry.Metadata.MachineName)
	}

	if errors.Is(entry.Err, api.ErrLogStreamStalled) {
		msg += " stopped responding"
	} else {
		msg += fmt.Sprintf(": %v", entry.Err)
	}

	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightYellow)
	fmt.Fprintln(os.Stderr, style.Render(msg))
}

// palette is available colors for machine/service differentiation.
var palette = []color.Color{
	lipgloss.BrightGreen,
	lipgloss.BrightYellow,
	lipgloss.BrightBlue,
	lipgloss.BrightMagenta,
	lipgloss.BrightCyan,
	lipgloss.Green,
	lipgloss.Yellow,
	lipgloss.Blue,
	lipgloss.Magenta,
	lipgloss.Cyan,
}
