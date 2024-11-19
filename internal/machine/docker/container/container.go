package container

import "github.com/docker/docker/api/types"

type Container struct {
	types.Container
}

// TODO: implement health related methods.
