package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"github.com/moby/term"
)

type BuildOptions struct {
	Files    []string
	Profiles []string
	Services []string
	Push     bool
	NoCache  bool
}

// GetServicesThatNeedBuild returns a map of services that require building
func GetServicesThatNeedBuild(project *composetypes.Project) map[string]composetypes.ServiceConfig {
	servicesToBuild := make(map[string]composetypes.ServiceConfig, len(project.Services))
	for serviceName, service := range project.Services {
		if service.Build == nil {
			continue
		}
		servicesToBuild[serviceName] = service
	}
	return servicesToBuild
}

// BuildServices builds the services defined in the provided map.
func BuildServices(ctx context.Context, servicesToBuild map[string]composetypes.ServiceConfig, opts BuildOptions) error {
	fmt.Println("Building services...")

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
		imageName, err := buildSingleService(ctx, dockerCli, service, opts)
		if err != nil {
			return fmt.Errorf("build service %s: %w", service.Name, err)
		}
		serviceImages[service.Name] = imageName
	}
	fmt.Printf("Service images are built.\n")

	if opts.Push {
		err = pushServiceImages(ctx, dockerCli, serviceImages)
	}

	return err
}

// buildSingleService builds a single service using the Docker client and Compose libraries.
func buildSingleService(ctx context.Context, dockerCli *dockerclient.Client, service composetypes.ServiceConfig, opts BuildOptions) (string, error) {
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
		NoCache:    opts.NoCache,
	}

	buildResponse, err := dockerCli.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return "", fmt.Errorf("failed to build image for service %s: %w", service.Name, err)
	}
	defer buildResponse.Body.Close()

	// Display the build response
	fd, isTerminal := term.GetFdInfo(os.Stdout)
	if err := jsonmessage.DisplayJSONMessagesStream(buildResponse.Body, os.Stdout, fd, isTerminal, nil); err != nil {
		return "", fmt.Errorf("failed to display build response for service %s: %w", service.Name, err)
	}

	return imageName, nil
}

// pushSingleServiceImage pushes a single service image.
func pushSingleServiceImage(ctx context.Context, dockerCli *dockerclient.Client, serviceName string, imageName string) error {
	ref, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return err
	}

	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return err
	}

	registryKey := repoInfo.Index.Name
	if repoInfo.Index.Official {
		registryKey = registry.IndexServer
	}

	// Load the Docker config file with auth details, if available
	configFile := config.LoadDefaultConfigFile(os.Stderr)

	authConfig, err := configFile.GetAuthConfig(registryKey)
	if err != nil {
		return err
	}

	authJSON, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal auth config for registry %s: %w", registryKey, err)
	}

	authStr := base64.URLEncoding.EncodeToString(authJSON)

	pushOptions := image.PushOptions{
		RegistryAuth: authStr,
	}
	pushResponse, err := dockerCli.ImagePush(ctx, imageName, pushOptions)
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", imageName, err)
	}
	defer pushResponse.Close()

	fmt.Printf("Pushing image %s for service %s...\n", imageName, serviceName)

	fd, isTerminal := term.GetFdInfo(os.Stdout)
	if err := jsonmessage.DisplayJSONMessagesStream(pushResponse, os.Stdout, fd, isTerminal, nil); err != nil {
		return fmt.Errorf("failed to display push response for image %s: %w", imageName, err)
	}

	fmt.Printf("Image %s pushed successfully.\n", imageName)
	return nil
}

// pushServiceImages pushes all built service images to the registry.
func pushServiceImages(ctx context.Context, dockerCli *dockerclient.Client, serviceImages map[string]string) error {
	fmt.Printf("Pushing images...\n")
	for serviceName, imageName := range serviceImages {
		if err := pushSingleServiceImage(ctx, dockerCli, serviceName, imageName); err != nil {
			return fmt.Errorf("push image for service %s: %w", serviceName, err)
		}
	}
	return nil
}
