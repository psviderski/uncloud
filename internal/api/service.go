package api

const (
	ServiceModeReplicated = "replicated"
	ServiceModeGlobal     = "global"
)

type ServiceSpec struct {
	Container ContainerSpec
	// Mode is the replication mode of the service. Default is ServiceModeReplicated if empty.
	Mode string
	Name string
}

type ContainerSpec struct {
	Command []string
	Image   string
	// Run a custom init inside the container. If nil, use the daemon's configured settings.
	Init *bool
}

type MachineContainerID struct {
	MachineID   string
	ContainerID string
}
