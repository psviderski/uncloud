package service

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"slices"
	"strings"
	"uncloud/internal/api"
	"uncloud/internal/cli"
	"uncloud/internal/cli/client"
	"uncloud/internal/machine/api/pb"
)

type runOptions struct {
	command           []string
	entrypoint        string
	entrypointChanged bool
	image             string
	machines          []string
	mode              string
	name              string
	publish           []string
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
	cmd.Flags().StringVar(&opts.mode, "mode", api.ServiceModeReplicated,
		fmt.Sprintf("Replication mode of the service: either %q (a specified number of containers across "+
			"the machines) or %q (one container on every machine).",
			api.ServiceModeReplicated, api.ServiceModeGlobal))
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Placement constraint by machine name, limiting which machines the service can run on. Can be specified "+
			"multiple times or as a comma-separated list of machine names. (default is any suitable machine)")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().StringSliceVarP(&opts.publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times.\n"+
			"Format: [hostname:][load_balancer_port:]container_port[/protocol] or [host_ip:]:host_port:container_port[/protocol]@host\n"+
			"Supported protocols: tcp, udp, http, https (default is tcp). If a hostname for http(s) port is not specified,\n"+
			"service-name.cluster-domain will be used as the hostname.\n"+
			"Examples:\n"+
			"  -p 8080/https                  Publish port 8080 as HTTPS via load balancer with default service-name.cluster-domain hostname\n"+
			"  -p app.example.com:8080/https  Publish port 8080 as HTTPS via load balancer with custom hostname\n"+
			"  -p 9000:8080                   Publish port 8080 as TCP port 9000 via load balancer\n"+
			"  -p 53:5353/udp@host            Bind UDP port 5353 to host port 53")
	cmd.Flags().UintVar(&opts.replicas, "replicas", 1,
		"Number of containers to run for the service. Only valid for a replicated service.")
	cmd.Flags().StringSliceVarP(&opts.volumes, "volume", "v", nil,
		"Bind mount a host file or directory into a service container using the format "+
			"/host/path:/container/path[:ro]. Can be specified multiple times.")

	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to run the service in. (default is the current cluster)",
	)

	return cmd
}

func run(ctx context.Context, uncli *cli.CLI, opts runOptions) error {
	switch opts.mode {
	case api.ServiceModeReplicated, api.ServiceModeGlobal:
	default:
		return fmt.Errorf("invalid replication mode: %q", opts.mode)
	}

	var machineFilter client.MachineFilter
	if len(opts.machines) > 0 {
		var machines []string
		for _, value := range opts.machines {
			if value == "" {
				continue
			}

			mlist := strings.Split(value, ",")
			for _, m := range mlist {
				if m = strings.TrimSpace(m); m != "" {
					machines = append(machines, m)
				}
			}
		}

		if len(machines) > 0 {
			machineFilter = func(m *pb.MachineInfo) bool {
				return slices.Contains(machines, m.Name)
			}
		}
	}

	ports := make([]api.PortSpec, len(opts.publish))
	for i, publishPort := range opts.publish {
		port, err := api.ParsePortSpec(publishPort)
		if err != nil {
			return fmt.Errorf("invalid service port '%s': %w", publishPort, err)
		}
		ports[i] = port
	}
	// TODO: parse and validate opts.volumes to fail fast if invalid.

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command: opts.command,
			Image:   opts.image,
			Volumes: opts.volumes,
		},
		Mode:     opts.mode,
		Name:     opts.name,
		Ports:    ports,
		Replicas: opts.replicas,
	}

	// Overwrite the default ENTRYPOINT of the image or reset it if an empty string is passed.
	if opts.entrypoint != "" {
		spec.Container.Entrypoint = []string{opts.entrypoint}
	} else if opts.entrypointChanged {
		spec.Container.Entrypoint = []string{""}
	}

	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid service configuration: %w", err)
	}

	clusterClient, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	resp, err := clusterClient.RunService(ctx, spec, machineFilter)
	if err != nil {
		return fmt.Errorf("run service: %w", err)
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
