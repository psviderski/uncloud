package constants

import (
	"math"
	"time"
)

const (
	// MachineAPIPort is the port for the Machine API service on the management WireGuard network.
	MachineAPIPort = 51000
	// UnregistryPort is the port for the embedded container registry listening on the machine IP.
	UnregistryPort = 5000
	// UnknownRTT is a sentinel duration used to sort peers with unknown RTT last.
	UnknownRTT = time.Duration(math.MaxInt64)
)
