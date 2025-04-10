package client

import (
	"context"

	dockerclient "github.com/docker/docker/client"
	"github.com/psviderski/uncloud/pkg/api"
)

func (cli *Client) InspectImage(ctx context.Context, id string) ([]api.MachineImage, error) {
	images, err := cli.Docker.InspectImage(ctx, id)
	if dockerclient.IsErrNotFound(err) {
		err = api.ErrNotFound
	}

	return images, err
}

func (cli *Client) InspectRemoteImage(ctx context.Context, id string) ([]api.MachineRemoteImage, error) {
	return cli.Docker.InspectRemoteImage(ctx, id)
}
