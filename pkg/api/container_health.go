package api

import "time"

const (
	// defaultDockerHealthcheckInterval is the default Docker interval between health check runs.
	defaultDockerHealthcheckInterval = 30 * time.Second
	// defaultDockerHealthcheckRetries is the default Docker number of consecutive failures needed
	// to consider the container unhealthy.
	defaultDockerHealthcheckRetries = 3
)
