package constants

const (
	// MachineAPIPort is the port for the Machine API service on the management WireGuard network.
	MachineAPIPort = 51000
	// UnregistryPort is the port for the embedded container registry listening on the machine IP.
	UnregistryPort = 5000

	// DefaultMachineSockPath is the default path to the machine API Unix socket.
	DefaultMachineSockPath = "/run/uncloud/machine.sock"
	// DefaultUncloudSockPath is the default path to the Uncloud API Unix socket.
	DefaultUncloudSockPath = "/run/uncloud/uncloud.sock"
	// DefaultSockGroup is the Linux group that owns the API sockets, granting access without root.
	DefaultSockGroup = "uncloud"
)
