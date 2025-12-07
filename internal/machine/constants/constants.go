package constants

const (
	// MachineAPIPort is the port for the Machine API service on the management WireGuard network.
	MachineAPIPort = 51000
	// UnregistryPort is the port for the embedded container registry listening on the machine IP.
	UnregistryPort = 5000
	// DefaultUncloudSockPath is the default path for the Unix socket used by the daemon for local communication.
	DefaultUncloudSockPath = "/run/uncloud/uncloud.sock"
)
