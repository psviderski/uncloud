package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

// FIXME: This is a copy of the deployOptions struct. It should be refactored to avoid duplication.
type buildOptions = deployOptions

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
	cmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context to deploy to (default is the current context)")

	return cmd
}

func runBuild(ctx context.Context, uncli *cli.CLI, opts buildOptions) error {
	fmt.Println("Building services...")
	fmt.Printf("Context: %#v\n", ctx)
	fmt.Printf("CLI: %#v\n", uncli)
	fmt.Printf("Opts: %#v\n", opts)

	projectOpts := projectOpts(opts)

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

	// TODO: figure out service dependencies?

	// Init local client
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer dockerCli.Close()

	// Build the services using the local docker client and compose libraries
	for _, service := range project.Services {
		fmt.Printf("Building service: %s\n", service.Name)
		fmt.Printf("Service.Build: %#v\n", service.Build)

		buildContextPath := service.Build.Context

		// Create a tar archive of the build context
		buildContext, err := archive.TarWithOptions(buildContextPath, &archive.TarOptions{})
		if err != nil {
			return fmt.Errorf("failed to create build context for service %s: %w", service.Name, err)
		}

		// TODO: use the proper project name
		imageName := api.GetImageNameOrDefault(service, "uncloud-default")
		// Set build options
		buildOptions := types.ImageBuildOptions{
			// TODO: Support Dockerfiles outside the build context
			// See https://github.com/docker/compose/blob/cf89fd1aa1328d5af77658ccc5a1e1b29981ae80/pkg/compose/build_classic.go#L92
			Dockerfile: service.Build.Dockerfile,
			Tags:       []string{imageName},
			Remove:     true, // Remove intermediate containers
		}

		fmt.Printf("Build options: %#v\n", buildOptions)

		// Build the image
		buildResponse, err := dockerCli.ImageBuild(context.Background(), buildContext, buildOptions)
		if err != nil {
			return fmt.Errorf("failed to build image for service %s: %w", service.Name, err)
		}
		defer buildResponse.Body.Close()

		// Read and display the build output
		_, err = io.Copy(os.Stdout, buildResponse.Body)
		if err != nil {
			return fmt.Errorf("failed to read build output for service %s: %w", service.Name, err)
		}
	}

	return nil
}

func buildService(ctx context.Context, dockerCli *dockerclient.Client, service composetypes.ServiceConfig) error {
	// build a service and return the image id/digest
	return nil
}

func pushImage(ctx context.Context, dockerCli *dockerclient.Client, imageName string) error {
	// push an image to a registry
	// This is a placeholder function. Implement the actual push logic here.
	fmt.Printf("Pushing image: %s\n", imageName)
	return nil
}

func absoluteDockerfilePath(buildContextPath, dockerfilePath string) string {
	if dockerfilePath == "" {
		return ""
	}

	if filepath.IsAbs(dockerfilePath) {
		return dockerfilePath
	}
	return filepath.Join(buildContextPath, dockerfilePath)
}
