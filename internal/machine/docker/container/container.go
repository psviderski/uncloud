package container

import "github.com/docker/docker/api/types"

const (
	LabelServiceID   = "uncloud.service.id"
	LabelServiceName = "uncloud.service.name"
)

type Container struct {
	types.Container
}

// TODO: implement health related methods.
