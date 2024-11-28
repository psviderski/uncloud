package ucind

import (
	"errors"
	"github.com/docker/docker/client"
)

var ErrNotFound = errors.New("not found")

type Provisioner struct {
	client *client.Client
}

func NewProvisioner(cli *client.Client) *Provisioner {
	return &Provisioner{
		client: cli,
	}
}
