package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

type buildOptions struct {
	files    []string
	profiles []string
	services []string
	push     bool
	noCache  bool
}

func NewBuildCommand() *cobra.Command {
	opts := buildOptions{}
	cmd := &cobra.Command{
		Use:   "build [FLAGS] [SERVICE...]",
		Short: "Build services FIXME.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.services = args
			}

			return runBuild(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil,
		"One or more Compose files to build (default compose.yaml)")
	cmd.Flags().StringSliceVarP(&opts.profiles, "profile", "p", nil,
		"One or more Compose profiles to enable.")
	cmd.Flags().BoolVarP(&opts.push, "push", "P", false,
		"Push built images to the registry after building. (default false)")
	cmd.Flags().BoolVarP(&opts.noCache, "no-cache", "n", false,
		"Do not use cache when building images. (default false)")

	return cmd
}

// TODO: deduplicate with deploy options
// projectOpts returns the project options for the Compose file(s).
func projectOptsFromBuildOpts(opts buildOptions) []composecli.ProjectOptionsFn {
	projectOpts := []composecli.ProjectOptionsFn{}

	if len(opts.profiles) > 0 {
		projectOpts = append(projectOpts, composecli.WithDefaultProfiles(opts.profiles...))
	}

	return projectOpts
}

// runBuild parses the Compose file(s), builds the services, and pushes them if requested.
func runBuild(ctx context.Context, uncli *cli.CLI, opts buildOptions) error {
	fmt.Println("Building services...")
	projectOpts := projectOptsFromBuildOpts(opts)
	project, err := compose.LoadProject(ctx, opts.files, projectOpts...)
	if err != nil {
		return fmt.Errorf("load compose file(s): %w", err)
	}

	if len(opts.services) > 0 {
		// Includes service dependencies by default. This is the default docker compose behavior.
		project, err = project.WithSelectedServices(opts.services)
		if err != nil {
			return fmt.Errorf("select services: %w", err)
		}
	}

	servicesToBuild := make(map[string]composetypes.ServiceConfig, len(project.Services))
	for serviceName, service := range project.Services {
		if service.Build == nil {
			continue
		}
		servicesToBuild[serviceName] = service
	}

	if len(servicesToBuild) == 0 {
		fmt.Println("No services to build.")
		return nil
	}

	// Init docker client (can be local or remote, depending on DOCKER_HOST environment variable)
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer dockerCli.Close()

	serviceImages := make(map[string]string, len(servicesToBuild))

	// Build the services using the local docker client and compose libraries
	for _, service := range servicesToBuild {
		fmt.Printf("Building service: %s\n", service.Name)
		imageName, err := buildService(ctx, dockerCli, service, opts)
		if err != nil {
			return fmt.Errorf("build service %s: %w", service.Name, err)
		}
		serviceImages[service.Name] = imageName
	}
	fmt.Printf("Service images are built.\n")

	if opts.push {
		return pushServiceImages(ctx, dockerCli, serviceImages)
	}
	return nil
}

// buildService builds a single service using the Docker client and Compose libraries.
func buildService(ctx context.Context, dockerCli *dockerclient.Client, service composetypes.ServiceConfig, opts buildOptions) (string, error) {
	if service.Build == nil {
		return "", fmt.Errorf("service %s has no build configuration", service.Name)
	}
	if service.Image == "" {
		return "", fmt.Errorf("service %s has no image specified; building services without image is not supported yet", service.Name)
	}

	buildContextPath := service.Build.Context
	imageName := service.Image

	// Create a tar archive of the build context
	buildContext, err := archive.TarWithOptions(buildContextPath, &archive.TarOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create build context for service %s: %w", service.Name, err)
	}

	buildOptions := types.ImageBuildOptions{
		// TODO: Support Dockerfiles outside the build context
		// See https://github.com/docker/compose/blob/cf89fd1aa1328d5af77658ccc5a1e1b29981ae80/pkg/compose/build_classic.go#L92
		Dockerfile: service.Build.Dockerfile,
		Tags:       []string{imageName},
		Remove:     true, // Remove intermediate containers
		NoCache:    opts.noCache,
	}

	buildResponse, err := dockerCli.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return "", fmt.Errorf("failed to build image for service %s: %w", service.Name, err)
	}
	defer buildResponse.Body.Close()

	// Print the build output
	decoder := json.NewDecoder(buildResponse.Body)
	for {
		var message map[string]interface{}
		if err := decoder.Decode(&message); err == io.EOF {
			break
		} else if err != nil {
			return "", fmt.Errorf("failed to decode build output for service %s: %w", service.Name, err)
		}

		if stream, ok := message["stream"]; ok {
			fmt.Print(stream)
		}
	}

	return imageName, nil
}

// pushServiceImage pushes a single service image.
func pushServiceImage(ctx context.Context, dockerCli *dockerclient.Client, serviceName string, imageName string) error {
	pushOptions := image.PushOptions{
		RegistryAuth: "dummy", // TODO: Handle authentication if needed
		All:          true,    // Push all tags
	}
	pushResponse, err := dockerCli.ImagePush(ctx, imageName, pushOptions)
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", imageName, err)
	}
	defer pushResponse.Close()

	fmt.Printf("Pushing image %s for service %s\n", imageName, serviceName)

	// Wait for push to complete
	if _, err := io.Copy(io.Discard, pushResponse); err != nil {
		return fmt.Errorf("read push response: %w", err)
	}

	fmt.Printf("Image %s pushed successfully.\n", imageName)
	return nil
}

// pushServiceImages pushes all built service images to the registry.
func pushServiceImages(ctx context.Context, dockerCli *dockerclient.Client, serviceImages map[string]string) error {
	fmt.Printf("Pushing images...\n")
	for serviceName, imageName := range serviceImages {
		if err := pushServiceImage(ctx, dockerCli, serviceName, imageName); err != nil {
			return fmt.Errorf("push image for service %s: %w", serviceName, err)
		}
	}
	return nil
}
