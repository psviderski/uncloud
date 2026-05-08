package completion

import (
	"context"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

// Images allows for completion of remote images.
func Images(ctx context.Context, uncli *cli.CLI, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer client.Close()

	machineImages, err := client.ListImages(ctx, api.ImageFilter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := []cobra.Completion{}
	for _, machineImage := range machineImages {
		for _, image := range machineImage.Images {
			if len(image.RepoTags) > 0 {
				if slices.Contains(args, image.RepoTags[0]) {
					continue
				}
				if strings.HasPrefix(image.RepoTags[0], toComplete) {
					names = append(names, image.RepoTags[0])
				}
			}
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// LocalImages allows for local image completion.
func LocalImages(ctx context.Context, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	dockerCliWrapped, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	dockerCli := &docker.Client{Client: dockerCliWrapped}
	defer dockerCli.Close()

	images, err := dockerCli.ImageList(context.Background(), image.ListOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	names := []cobra.Completion{}
	for _, image := range images {
		if len(image.RepoTags) > 0 {
			if slices.Contains(args, image.RepoTags[0]) {
				continue
			}
			if strings.HasPrefix(image.RepoTags[0], toComplete) {
				names = append(names, image.RepoTags[0])
			}
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp

}
