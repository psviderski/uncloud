package logs

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/docker/docker/pkg/stringid"
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

		style = style.Foreground(Palette[i%len(Palette)])
	}

	return style.Render(name)
}

func (f *Formatter) formatServiceContainer(serviceName, containerID string) string {
	styleService := lipgloss.NewStyle().Bold(true).PaddingRight(f.maxServiceWidth - len(serviceName))
	styleContainer := lipgloss.NewStyle().Faint(true)

	if len(f.serviceNames) > 1 {
		// Service name is coloured for multi-service logs.
		i := slices.Index(f.serviceNames, serviceName)
		if i == -1 {
			f.serviceNames = append(f.serviceNames, serviceName)
			i = len(f.serviceNames) - 1
		}

		styleService = styleService.Foreground(Palette[i%len(Palette)])
	}

	return styleService.Render(serviceName) + styleContainer.Render("["+containerID[:5]+"]")
}

// printEntry prints a single log entry with proper formatting.
func (f *Formatter) PrintEntry(entry api.ServiceLogEntry) {
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

	// Service[container_id]
	output.WriteString(f.formatServiceContainer(entry.Metadata.ServiceName, entry.Metadata.ContainerID))
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

// PrintError prints an error entry (e.g., stalled stream warning).
func (f *Formatter) PrintError(entry api.ServiceLogEntry) {
	if entry.Metadata.ContainerID != "" {
		msg := fmt.Sprintf("WARNING: log stream from %s[%s] on machine '%s'",
			entry.Metadata.ServiceName,
			stringid.TruncateID(entry.Metadata.ContainerID),
			entry.Metadata.MachineName)

		if errors.Is(entry.Err, api.ErrLogStreamStalled) {
			msg += " stopped responding"
		} else {
			msg += fmt.Sprintf(": %v", entry.Err)
		}

		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // Bold bright yellow
		fmt.Fprintln(os.Stderr, style.Render(msg))
	} else {
		msg := fmt.Sprintf("ERROR: %v", entry.Err)
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // Bold bright red
		fmt.Fprintln(os.Stderr, style.Render(msg))
	}
}
