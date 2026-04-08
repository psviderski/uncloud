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

// LogStreamTypeFromProto converts a protobuf LogEntry.StreamType to the internal LogStreamType.
func LogStreamTypeFromProto(s pb.LogEntry_StreamType) LogStreamType {
	switch s {
	case pb.LogEntry_STDOUT:
		return LogStreamStdout
	case pb.LogEntry_STDERR:
		return LogStreamStderr
	case pb.LogEntry_HEARTBEAT:
		return LogStreamHeartbeat
	default:
		return LogStreamUnknown
	}
}

// LogStreamTypeToProto converts LogStreamType to protobuf LogEntry.StreamType.
func LogStreamTypeToProto(s LogStreamType) pb.LogEntry_StreamType {
	switch s {
	case LogStreamStdout:
		return pb.LogEntry_STDOUT
	case LogStreamStderr:
		return pb.LogEntry_STDERR
	case LogStreamHeartbeat:
		return pb.LogEntry_HEARTBEAT
	default:
		return pb.LogEntry_UNKNOWN
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
	LogEntry
}

// ServiceLogEntryMetadata contains metadata about the source of a log entry.
type ServiceLogEntryMetadata struct {
	ServiceID   string
	ServiceName string
	ContainerID string
	MachineID   string
	MachineName string
}

// LogEntry represents a single log entry from a container or a service.
type LogEntry struct {
	Stream    LogStreamType
	Timestamp time.Time
	Message   []byte
	// Err indicates that an error occurred while streaming logs from a container.
	// Other fields are not set if Err is not nil.
	Err error
}

// ErrLogStreamStalled indicates that a log stream stopped sending data and may be unresponsive.
var ErrLogStreamStalled = errors.New("log stream stopped responding")
