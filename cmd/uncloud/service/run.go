package service

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/daemon/names"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/spf13/cobra"
)

type runOptions struct {
	command           []string
	entrypoint        string
	entrypointChanged bool
	env               []string
	image             string
	machines          []string
	mode              string
	name              string
	publish           []string
	pull              string
	replicas          uint
	volumes           []string

	cluster string
}

func NewRunCommand() *cobra.Command {
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
	}

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
		"Placement constraint by machine name, limiting which machines the service can run on. Can be specified "+
			"multiple times or as a comma-separated list of machine names. (default is any suitable machine)")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().StringSliceVarP(&opts.publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times.\n"+
			"Format: [hostname:][load_balancer_port:]container_port[/protocol] or [host_ip:]:host_port:container_port[/protocol]@host\n"+
			"Supported protocols: tcp, udp, http, https (default is tcp). If a hostname for http(s) port is not specified\n"+
			"and a cluster domain is reserved, service-name.cluster-domain will be used as the hostname.\n"+
			"Examples:\n"+
			"  -p 8080/https                  Publish port 8080 as HTTPS via load balancer with default service-name.cluster-domain hostname\n"+
			"  -p app.example.com:8080/https  Publish port 8080 as HTTPS via load balancer with custom hostname\n"+
			"  -p 9000:8080                   Publish port 8080 as TCP port 9000 via load balancer\n"+
			"  -p 53:5353/udp@host            Bind UDP port 5353 to host port 53")
	cmd.Flags().StringVar(&opts.pull, "pull", api.PullPolicyMissing,
		fmt.Sprintf("Pull image from the registry before running service containers ('%s', '%s', '%s').",
			api.PullPolicyAlways, api.PullPolicyMissing, api.PullPolicyNever))
	cmd.Flags().UintVar(&opts.replicas, "replicas", 1,
		"Number of containers to run for the service. Only valid for a replicated service.")
	cmd.Flags().StringSliceVarP(&opts.volumes, "volume", "v", nil,
		"Mount a data volume or host path into service containers. Service containers will be scheduled on the machine(s) where\n"+
			"the volume is located. Can be specified multiple times.\n"+
			"Format: volume_name:/container/path[:ro|volume-nocopy] or /host/path:/container/path[:ro]\n"+
			"Examples:\n"+
			"  -v postgres-data:/var/lib/postgresql/data  Mount volume 'postgres-data' to /var/lib/postgresql/data in container\n"+
			"  -v /data/uploads:/app/uploads         	 Bind mount /data/uploads host directory to /app/uploads in container\n"+
			"  -v /host/path:/container/path:ro 		 Bind mount a host directory or file as read-only")

	cmd.Flags().StringVarP(
		&opts.cluster, "context", "c", "",
		"Name of the cluster context to run the service in. (default is the current context)",
	)

	return cmd
}

func run(ctx context.Context, uncli *cli.CLI, opts runOptions) error {
	spec, err := prepareServiceSpec(opts)
	if err != nil {
		return err
	}

	clusterClient, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	var deployFilter deploy.MachineFilter
	machines := cli.ExpandCommaSeparatedValues(opts.machines)
	if len(machines) > 0 {
		deployFilter = func(m *pb.MachineInfo) bool {
			return slices.Contains(machines, m.Name)
		}
	}

	machineIDForVolumes, missingVolumes, err := selectMachineForVolumes(ctx, clusterClient, spec.Volumes, machines)
	if err != nil {
		return err
	}
	// machineIDForVolumes is not empty if the spec includes named volumes.
	if machineIDForVolumes != "" {
		// The service must be deployed on the machine where the existing volumes are located and the missing ones
		// will be created.
		deployFilter = func(m *pb.MachineInfo) bool {
			return m.Id == machineIDForVolumes
		}
	}

	var resp client.RunServiceResponse
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		// Create missing volumes on the selected machine.
		for _, v := range missingVolumes {
			_, err = clusterClient.CreateVolume(ctx, machineIDForVolumes, volume.CreateOptions{Name: v.Name})
			if err != nil {
				return fmt.Errorf("create volume '%s': %w", v.Name, err)
			}
		}

		resp, err = clusterClient.RunService(ctx, spec, deployFilter)
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

	spec = api.ServiceSpec{
		Container: api.ContainerSpec{
			Command:      opts.command,
			Env:          env,
			Image:        opts.image,
			PullPolicy:   opts.pull,
			VolumeMounts: mounts,
		},
		Mode:     opts.mode,
		Name:     opts.name,
		Ports:    ports,
		Replicas: opts.replicas,
		Volumes:  volumes,
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

// selectMachineForVolumes selects a machine to run a service with the given volumes on and determines which volumes
// need to be created on the selected machine. An empty machineID is returned if no named volumes are specified.
func selectMachineForVolumes(
	ctx context.Context, clusterClient *client.Client, volumes []api.VolumeSpec, machinesFilter []string,
) (machineID string, missingVolumes []api.VolumeSpec, err error) {
	var volumeNames []string
	for _, volume := range volumes {
		if volume.Type == api.VolumeTypeVolume {
			volumeNames = append(volumeNames, volume.Name)
		}
	}
	if len(volumeNames) == 0 {
		return "", nil, nil
	}

	vfilter := &api.VolumeFilter{
		Machines: machinesFilter,
		Names:    volumeNames,
	}
	vols, err := clusterClient.ListVolumes(ctx, vfilter)
	if err != nil {
		return "", nil, fmt.Errorf("list volumes: %w", err)
	}

	if len(vols) > 0 {
		// Some volumes have been found on the machines matching the machinesFilter.
		// Pick the machine with the most volumes to create fewer duplicate volumes.
		volumesCountOnMachines := make(map[string]int)
		for _, vol := range vols {
			volumesCountOnMachines[vol.MachineID]++
		}

		maxCount := 0
		for mid, count := range volumesCountOnMachines {
			if count > maxCount {
				machineID = mid
				maxCount = count
			}
		}
	} else {
		// No volumes found on the machines matching the machinesFilter.
		// Pick the first available machine to create the volumes on.
		mfilter := &api.MachineFilter{
			Available:  true,
			NamesOrIDs: machinesFilter,
		}
		availableMachines, err := clusterClient.ListMachines(ctx, mfilter)
		if err != nil {
			return "", nil, fmt.Errorf("list machines: %w", err)
		}

		if len(availableMachines) == 0 {
			return "", nil, fmt.Errorf("no available machines to create the volume(s) on")
		}
		machineID = availableMachines[0].Machine.Id
	}

	// Find missing volumes that need to be created on the selected machine.
	for _, volume := range volumes {
		if volume.Type != api.VolumeTypeVolume {
			continue
		}

		if !slices.ContainsFunc(vols, func(v api.MachineVolume) bool {
			return v.Volume.Name == volume.Name && v.MachineID == machineID
		}) {
			missingVolumes = append(missingVolumes, volume)
		}
	}

	return machineID, missingVolumes, nil
}
