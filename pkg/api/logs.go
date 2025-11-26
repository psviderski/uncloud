package api

import (
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

// ServiceLogsOptions specifies parameters for ServiceLogs.
type ServiceLogsOptions struct {
	Follow bool
	Tail   int
	Since  string
	Until  string
}

// ServiceLogEntry represents a single log entry from a service container.
type ServiceLogEntry struct {
	Metadata  ServiceLogEntryMetadata
	Stream    LogStreamType
	Message   []byte
	Timestamp time.Time
	// Err indicates that an error occurred while streaming logs from a container.
	// Other non-Metadata fields are not set if Err is not nil.
	Err error
}

// ServiceLogEntryMetadata contains metadata about the source of a log entry.
type ServiceLogEntryMetadata struct {
	ServiceID     string
	ServiceName   string
	ContainerID   string
	ContainerName string
	MachineID     string
	MachineName   string
}
