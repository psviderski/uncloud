package api

import (
	"errors"
	"time"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

const (
	LogStreamUnknown LogStreamType = iota
	LogStreamStdout
	LogStreamStderr
	// LogStreamHeartbeat represents a heartbeat log entry with a timestamp indicating that
	// there are no older logs than this timestamp.
	LogStreamHeartbeat
)

type LogStreamType int

// LogStreamTypeFromProto converts a protobuf ContainerLogEntry.StreamType to the internal LogStreamType.
func LogStreamTypeFromProto(s pb.ContainerLogEntry_StreamType) LogStreamType {
	switch s {
	case pb.ContainerLogEntry_STDOUT:
		return LogStreamStdout
	case pb.ContainerLogEntry_STDERR:
		return LogStreamStderr
	case pb.ContainerLogEntry_HEARTBEAT:
		return LogStreamHeartbeat
	default:
		return LogStreamUnknown
	}
}

// LogStreamTypeToProto converts LogStreamType to protobuf ContainerLogEntry.StreamType.
func LogStreamTypeToProto(s LogStreamType) pb.ContainerLogEntry_StreamType {
	switch s {
	case LogStreamStdout:
		return pb.ContainerLogEntry_STDOUT
	case LogStreamStderr:
		return pb.ContainerLogEntry_STDERR
	case LogStreamHeartbeat:
		return pb.ContainerLogEntry_HEARTBEAT
	default:
		return pb.ContainerLogEntry_UNKNOWN
	}
}

// ServiceLogsOptions specifies parameters for ServiceLogs.
type ServiceLogsOptions struct {
	Follow bool
	Tail   int
	Since  string
	Until  string
	// Machines filters logs to only include containers running on the specified machines (names or IDs).
	// If empty, logs from all machines are included.
	Machines []string
}

// ServiceLogEntry represents a single log entry from a service container.
type ServiceLogEntry struct {
	// Metadata may not be set if an error occurred (Err is not nil).
	Metadata ServiceLogEntryMetadata
	ContainerLogEntry
}

// ServiceLogEntryMetadata contains metadata about the source of a log entry.
type ServiceLogEntryMetadata struct {
	ServiceID   string
	ServiceName string
	ContainerID string
	MachineID   string
	MachineName string
}

// ContainerLogEntry represents a single log entry from a container.
type ContainerLogEntry struct {
	Stream    LogStreamType
	Timestamp time.Time
	Message   []byte
	// Err indicates that an error occurred while streaming logs from a container.
	// Other fields are not set if Err is not nil.
	Err error
}

// ErrLogStreamStalled indicates that a log stream stopped sending data and may be unresponsive.
var ErrLogStreamStalled = errors.New("log stream stopped responding")
