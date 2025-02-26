package service

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/api"
	"uncloud/internal/cli"
)

type runOptions struct {
	command []string
	image   string
	machine string
	mode    string
	name    string
	publish []string
	volumes []string

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

			opts.image = args[0]
			if len(args) > 1 {
				opts.command = args[1:]
			}

			return run(cmd.Context(), uncli, opts)
		},
	}

	// TODO: implement placement constraints and translate --machine to a constraint.
	//cmd.Flags().StringVarP(
	//	&opts.machine, "machine", "m", "",
	//	"Name or ID of the machine to run the service on. (default is first available)",
	//)
	cmd.Flags().StringVar(&opts.mode, "mode", api.ServiceModeReplicated,
		fmt.Sprintf("Replication mode of the service: either %q (a specified number of containers across "+
			"the machines) or %q (one container on every machine).",
			api.ServiceModeReplicated, api.ServiceModeGlobal))
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
	case "", api.ServiceModeReplicated, api.ServiceModeGlobal:
	default:
		return fmt.Errorf("invalid replication mode: %q", opts.mode)
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
		Mode:  opts.mode,
		Name:  opts.name,
		Ports: ports,
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid service configuration: %w", err)
	}

	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	resp, err := client.RunService(ctx, spec)
	if err != nil {
		return fmt.Errorf("run service: %w", err)
	}

	svc, err := client.InspectService(ctx, resp.ID)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s endpoints:\n", svc.Name)
	for _, endpoint := range svc.Endpoints() {
		fmt.Printf(" • %s\n", endpoint)
	}

	return nil
}
