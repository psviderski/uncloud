package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	dockeropts "github.com/docker/cli/opts"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/daemon/names"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/spf13/cobra"
)

type runOptions struct {
	caddyfile         string
	command           []string
	cpu               dockeropts.NanoCPUs
	entrypoint        string
	entrypointChanged bool
	env               []string
	image             string
	machines          []string
	memory            dockeropts.MemBytes
	mode              string
	name              string
	privileged        bool
	publish           []string
	pull              string
	replicas          uint
	user              string
	volumes           []string
}

func NewRunCommand(groupID string) *cobra.Command {
	opts := runOptions{}

	cmd := &cobra.Command{
		Use:   "run IMAGE [COMMAND...]",
		Short: "Run a service.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.entrypointChanged = cmd.Flag("entrypoint").Changed
			opts.image = args[0]
			if len(args) > 1 {
				opts.command = args[1:]
			}

			return run(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}

	cmd.Flags().StringVar(&opts.caddyfile, "caddyfile", "",
		"Path to a custom Caddy config (Caddyfile) for the service. "+
			"Cannot be used together with non-@host published ports.")
	cmd.Flags().VarP(&opts.cpu, "cpu", "",
		"Maximum number of CPU cores a service container can use. Fractional values are allowed: "+
			"0.5 for half a core or 2.25 for two and a quarter cores.")
	cmd.Flags().StringVar(&opts.entrypoint, "entrypoint", "",
		"Overwrite the default ENTRYPOINT of the image. Pass an empty string \"\" to reset it.")
	cmd.Flags().StringSliceVarP(&opts.env, "env", "e", nil,
		"Set an environment variable for service containers. Can be specified multiple times.\n"+
			"Format: VAR=value or just VAR to use the value from the local environment.")
	cmd.Flags().StringVar(&opts.mode, "mode", api.ServiceModeReplicated,
		fmt.Sprintf("Replication mode of the service: either '%s' (a specified number of containers across "+
			"the machines) or '%s' (one container on every machine).",
			api.ServiceModeReplicated, api.ServiceModeGlobal))
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Placement constraint by machine names, limiting which machines the service can run on. Can be specified "+
			"multiple times or as a comma-separated list of machine names. (default is any suitable machine)")
	cmd.Flags().VarP(&opts.memory, "memory", "",
		"Maximum amount of memory a service container can use. Value is a positive integer with optional unit suffix "+
			"(b, k, m, g). Default unit is bytes if no suffix specified.\n"+
			"Examples: 1073741824, 1024m, 1g (all equal 1 gibibyte)")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().BoolVar(&opts.privileged, "privileged", false,
		"Give extended privileges to service containers. This is a security risk and should be used with caution.")
	cmd.Flags().StringSliceVarP(&opts.publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times.\n"+
			"Format: [hostname:]container_port[/protocol] or [host_ip:]host_port:container_port[/protocol]@host\n"+
			"Supported protocols: tcp, udp, http, https (default is tcp). If a hostname for http(s) port is not specified\n"+
			"and a cluster domain is reserved, service-name.cluster-domain will be used as the hostname.\n"+
			"Examples:\n"+
			"  -p 8080/https                  Publish port 8080 as HTTPS via reverse proxy with default service-name.cluster-domain hostname\n"+
			"  -p app.example.com:8080/https  Publish port 8080 as HTTPS via reverse proxy with custom hostname\n"+
			// TODO: add support for publishing L4 tcp/udp ports.
			//"  -p 9000:8080                   Publish port 8080 as TCP port 9000 via reverse proxy\n"+
			"  -p 53:5353/udp@host            Bind UDP port 5353 to host port 53")
	cmd.Flags().StringVar(&opts.pull, "pull", api.PullPolicyMissing,
		fmt.Sprintf("Pull image from the registry before running service containers ('%s', '%s', '%s').",
			api.PullPolicyAlways, api.PullPolicyMissing, api.PullPolicyNever))
	cmd.Flags().UintVar(&opts.replicas, "replicas", 1,
		"Number of containers to run for the service. Only valid for a replicated service.")
	cmd.Flags().StringVarP(&opts.user, "user", "u", "",
		"User name or UID and optionally group name or GID used for running the command inside service containers.\n"+
			"Format: USER[:GROUP] or UID[:GID]. If not specified, the user is set to the default user of the image.")
	cmd.Flags().StringSliceVarP(&opts.volumes, "volume", "v", nil,
		"Mount a data volume or host path into service containers. Service containers will be scheduled on the machine(s) where\n"+
			"the volume is located. Can be specified multiple times.\n"+
			"Format: volume_name:/container/path[:ro|volume-nocopy] or /host/path:/container/path[:ro]\n"+
			"Examples:\n"+
			"  -v postgres-data:/var/lib/postgresql/data  Mount volume 'postgres-data' to /var/lib/postgresql/data in container\n"+
			"  -v /data/uploads:/app/uploads         	 Bind mount /data/uploads host directory to /app/uploads in container\n"+
			"  -v /host/path:/container/path:ro 		 Bind mount a host directory or file as read-only")

	return cmd
}

func run(ctx context.Context, uncli *cli.CLI, opts runOptions) error {
	spec, err := prepareServiceSpec(opts)
	if err != nil {
		return err
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	var resp api.RunServiceResponse
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		resp, err = clusterClient.RunService(ctx, spec)
		if err != nil {
			return fmt.Errorf("run service: %w", err)
		}

		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Running service %s (%s mode)", spec.Name, spec.Mode))
	if err != nil {
		return err
	}

	svc, err := clusterClient.InspectService(ctx, resp.ID)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	endpoints := svc.Endpoints()
	if len(endpoints) > 0 {
		fmt.Println()
		fmt.Printf("%s endpoints:\n", svc.Name)
		for _, endpoint := range endpoints {
			fmt.Printf(" • %s\n", endpoint)
		}
	}

	return nil
}

func prepareServiceSpec(opts runOptions) (api.ServiceSpec, error) {
	var spec api.ServiceSpec

	caddyfile := ""
	if opts.caddyfile != "" {
		data, err := os.ReadFile(opts.caddyfile)
		if err != nil {
			return spec, fmt.Errorf("read Caddyfile: %w", err)
		}
		caddyfile = strings.TrimSpace(string(data))
	}

	env, err := parseEnv(opts.env)
	if err != nil {
		return spec, err
	}

	switch opts.mode {
	case api.ServiceModeReplicated, api.ServiceModeGlobal:
	default:
		return spec, fmt.Errorf("invalid replication mode: '%s'", opts.mode)
	}

	switch opts.pull {
	case api.PullPolicyAlways, api.PullPolicyMissing, api.PullPolicyNever:
	default:
		return spec, fmt.Errorf("invalid pull policy: '%s'", opts.pull)
	}

	ports := make([]api.PortSpec, len(opts.publish))
	for i, publishPort := range opts.publish {
		port, err := api.ParsePortSpec(publishPort)
		if err != nil {
			return spec, fmt.Errorf("invalid service port '%s': %w", publishPort, err)
		}
		ports[i] = port
	}

	volumes, mounts, err := parseVolumeFlags(opts.volumes)
	if err != nil {
		return spec, err
	}

	placement := api.Placement{
		Machines: cli.ExpandCommaSeparatedValues(opts.machines),
	}

	spec = api.ServiceSpec{
		Container: api.ContainerSpec{
			Command:    opts.command,
			Env:        env,
			Image:      opts.image,
			Privileged: opts.privileged,
			PullPolicy: opts.pull,
			Resources: api.ContainerResources{
				CPU:    opts.cpu.Value(),
				Memory: opts.memory.Value(),
			},
			User:         opts.user,
			VolumeMounts: mounts,
		},
		Mode:      opts.mode,
		Name:      opts.name,
		Placement: placement,
		Ports:     ports,
		Replicas:  opts.replicas,
		Volumes:   volumes,
	}

	if caddyfile != "" {
		spec.Caddy = &api.CaddySpec{
			Config: caddyfile,
		}
	}

	// Overwrite the default ENTRYPOINT of the image or reset it if an empty string is passed.
	if opts.entrypoint != "" {
		spec.Container.Entrypoint = []string{opts.entrypoint}
	} else if opts.entrypointChanged {
		spec.Container.Entrypoint = []string{""}
	}

	if err = spec.Validate(); err != nil {
		return spec, fmt.Errorf("invalid service configuration: %w", err)
	}

	// Generate a service name if not specified to be able to include it in the progress title.
	if spec.Name == "" {
		spec.Name, err = deploy.GenerateServiceName(spec.Container.Image)
		if err != nil {
			return spec, fmt.Errorf("generate service name: %w", err)
		}
	}

	return spec, err
}

// parseEnv parses the environment variables from the command line arguments.
// It supports two formats: "VAR=value" or just "VAR" to use the value from the local environment.
func parseEnv(env []string) (api.EnvVars, error) {
	envVars := make(api.EnvVars)
	for _, e := range env {
		key, value, hasValue := strings.Cut(e, "=")
		if key == "" {
			return nil, fmt.Errorf("invalid environment variable: '%s'", e)
		}

		if hasValue {
			envVars[key] = value
		} else {
			if localEnvValue, ok := os.LookupEnv(key); ok {
				envVars[key] = localEnvValue
			}
		}
	}

	return envVars, nil
}

// parseVolumeFlags parses volume flag values in Docker CLI format and returns VolumeSpecs and VolumeMounts.
// It handles both named volumes (volume_name:/container/path[:ro|volume-nocopy])
// and bind mounts (/host/path:/container/path[:ro]).
func parseVolumeFlags(volumes []string) ([]api.VolumeSpec, []api.VolumeMount, error) {
	specs := make([]api.VolumeSpec, 0, len(volumes))
	mounts := make([]api.VolumeMount, 0, len(volumes))

	// Track volume names to avoid duplicate specs.
	seenVolumes := make(map[string]struct{})

	for _, vol := range volumes {
		spec, mount, err := parseVolumeFlagValue(vol)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid volume mount '%s': %w", vol, err)
		}

		if _, ok := seenVolumes[spec.Name]; !ok {
			specs = append(specs, spec)
			seenVolumes[spec.Name] = struct{}{}
		}

		mounts = append(mounts, mount)
	}

	return specs, mounts, nil
}

func parseVolumeFlagValue(volume string) (api.VolumeSpec, api.VolumeMount, error) {
	var spec api.VolumeSpec
	var mount api.VolumeMount

	parts := strings.Split(volume, ":")
	switch len(parts) {
	case 1:
		return spec, mount, fmt.Errorf("invalid format, must contain at least one separator ':'")
	case 2, 3:
		// Format: (volume_name|/host/path):/container/path[:opts]
		if !strings.HasPrefix(parts[1], "/") {
			return spec, mount, fmt.Errorf("invalid container mount path: '%s', must be absolute path", parts[1])
		}

		mount.ContainerPath = parts[1]
		volumeNoCopy := false

		if len(parts) == 3 {
			opts := strings.Split(parts[2], ",")
			for _, opt := range opts {
				switch opt {
				case "ro", "readonly":
					mount.ReadOnly = true
				case "volume-nocopy":
					volumeNoCopy = true
				default:
					return spec, mount, fmt.Errorf("invalid option: '%s'", opt)
				}
			}
		}

		if strings.HasPrefix(parts[0], "/") {
			// Host path bind mount: /host/path:/container/path
			suffix, err := secret.RandomAlphaNumeric(4)
			if err != nil {
				return spec, mount, fmt.Errorf("generate random suffix: %w", err)
			}

			spec = api.VolumeSpec{
				Name: "bind-" + suffix,
				Type: api.VolumeTypeBind,
				BindOptions: &api.BindOptions{
					HostPath:       parts[0],
					CreateHostPath: true,
				},
			}
		} else {
			// Named volume mount: volume_name:/container/path
			volumeName := parts[0]
			if !names.RestrictedNamePattern.MatchString(volumeName) {
				return spec, mount, fmt.Errorf("volume name '%s' includes invalid characters, only '%s' are allowed. "+
					"If you intended to pass a host directory or file, use absolute path",
					volumeName, names.RestrictedNameChars)
			}

			spec = api.VolumeSpec{
				Name: volumeName,
				Type: api.VolumeTypeVolume,
				VolumeOptions: &api.VolumeOptions{
					Name:   volumeName,
					NoCopy: volumeNoCopy,
				},
			}
		}

		mount.VolumeName = spec.Name
	default:
		return spec, mount, fmt.Errorf("invalid format, must container at most 2 separators ':'")
	}

	return spec, mount, nil
}
