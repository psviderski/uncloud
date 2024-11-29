package ucind

import (
	"errors"
	"github.com/docker/docker/client"
)

var ErrNotFound = errors.New("not found")

type Provisioner struct {
	dockerCli     *client.Client
	configUpdater *ConfigUpdater
}

func NewProvisioner(cli *client.Client, configUpdater *ConfigUpdater) *Provisioner {
	return &Provisioner{
		dockerCli:     cli,
		configUpdater: configUpdater,
	}
}
