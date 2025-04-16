package api

// Placement defines the placement constraints for service containers.
type Placement struct {
	// Machines is a list of machine names or IDs where service containers are allowed to be deployed.
	// If empty, containers can be deployed to any available machine in the cluster.
	Machines []string `json:",omitempty"`
}
