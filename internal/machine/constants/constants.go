package constants

const (
	// UncloudAPIPort is the TCP port on which each machine serves the Uncloud API over the management WireGuard
	// network.
	UncloudAPIPort = 51000
	// UnregistryPort is the TCP port for the embedded container registry listening on the machine IP.
	UnregistryPort = 51500
)
