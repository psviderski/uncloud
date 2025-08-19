package caddyconfig

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
)

const caddyfileTemplate = `http:// {
	handle {{.VerifyPath}} {
		respond "{{.VerifyResponse}}" 200
	}
	log
}

(common_proxy) {
	# Retry failed requests up to lb_retries times against other available upstreams.
	lb_retries 3
	# Upstreams are marked unhealthy for fail_duration after a failed request (passive health checking).
	fail_duration 30s
}
{{- range $hostname, $upstreams := .HTTPHostUpstreams}}

http://{{$hostname}} {
	reverse_proxy {
		to {{join $upstreams " "}}
		import common_proxy
	}
	log
}{{end}}
{{- range $hostname, $upstreams := .HTTPSHostUpstreams}}

https://{{$hostname}} {
	reverse_proxy {
		to {{join $upstreams " "}}
		import common_proxy
	}
	log
}{{end}}
`

// CaddyfileGenerator generates a Caddyfile configuration for the Caddy reverse proxy.
type CaddyfileGenerator struct {
	// machineID is the unique identifier of the machine where the controller is running.
	machineID string
	validator CaddyfileValidator
	log       *slog.Logger
}

// CaddyfileValidator is an interface for validating Caddyfile configurations.
type CaddyfileValidator interface {
	Validate(ctx context.Context, caddyfile string) error
}

func NewCaddyfileGenerator(machineID string, validator CaddyfileValidator, log *slog.Logger) *CaddyfileGenerator {
	if log == nil {
		log = slog.Default()
	}
	return &CaddyfileGenerator{
		machineID: machineID,
		validator: validator,
		log:       log,
	}
}

// Generate creates a Caddyfile configuration based on the provided service containers.
// If a 'caddy' service container is running on this machine and defines a custom Caddy config (x-caddy) in its service
// spec, it will be validated and prepended to the generated Caddyfile. Custom Caddy configs (x-caddy) defined in other
// service specs are validated and appended to the generated Caddyfile. Invalid configs are logged and skipped to ensure
// the generated Caddyfile remains valid.
//
// The final Caddyfile structure includes:
//
//	[caddy x-caddy]
//	[generated Caddyfile from all service ports]
//	[service-a x-caddy]
//	...
//	[service-z x-caddy]
func (g *CaddyfileGenerator) Generate(ctx context.Context, records []store.ContainerRecord) (string, error) {
	containers := make([]api.ServiceContainer, len(records))
	for i, cr := range records {
		containers[i] = cr.Container
	}
	// Sort containers by service name and container ID to generate a stable Caddyfile.
	slices.SortFunc(containers, func(a, b api.ServiceContainer) int {
		return cmp.Or(
			strings.Compare(a.ServiceName(), b.ServiceName()),
			strings.Compare(a.ID, b.ID),
		)
	})

	caddyfile, err := g.generateBaseFromPorts(containers)
	if err != nil {
		return "", fmt.Errorf("generate base Caddyfile from service ports: %w", err)
	}

	// Find the 'caddy' service container on this machine. Use the most recent one if multiple exist.
	var caddyCtr *api.ServiceContainer
	for _, cr := range records {
		if cr.MachineID == g.machineID && cr.Container.ServiceName() == CaddyServiceName &&
			(caddyCtr == nil || cr.Container.Created > caddyCtr.Created) {
			caddyCtr = &cr.Container
		}
	}

	// If the caddy container is running on this machine and has a custom Caddy config, prepend it to the generated
	// Caddyfile and validate it.
	if caddyCtr != nil && caddyCtr.ServiceSpec.CaddyConfig() != "" {
		// TODO: render the template actions in the Caddy config.
		caddyfileCandidate := caddyCtr.ServiceSpec.CaddyConfig() + "\n\n" + caddyfile

		if err = g.validator.Validate(ctx, caddyfileCandidate); err != nil {
			g.log.Error("Custom Caddy config on the caddy container on this machine is invalid, skipping it.",
				"service", caddyCtr.ServiceName(), "container", caddyCtr.ID, "err", err)
		} else {
			caddyfile = caddyfileCandidate
		}
	}

	// There could be multiple service containers for the same service with different custom Caddy configs, for example,
	// if the service has been partially updated. The most recent container for each service defines the current custom
	// Caddy config for that service.
	latestServiceContainers := make(map[string]api.ServiceContainer, len(containers))
	for _, ctr := range containers {
		if latest, ok := latestServiceContainers[ctr.ServiceName()]; ok {
			if ctr.Created > latest.Created {
				latestServiceContainers[ctr.ServiceName()] = ctr
			}
		} else {
			latestServiceContainers[ctr.ServiceName()] = ctr
		}
	}
	sortedServiceNames := slices.Sorted(maps.Keys(latestServiceContainers))

	for _, serviceName := range sortedServiceNames {
		// Skip the caddy container as we already processed it.
		if serviceName == CaddyServiceName {
			continue
		}

		ctr := latestServiceContainers[serviceName]
		if ctr.ServiceSpec.CaddyConfig() == "" {
			continue
		}

		// TODO: render the template actions in the Caddy config.
		caddyfileCandidate := fmt.Sprintf("%s\n# Service: %s\n%s\n",
			caddyfile, ctr.ServiceName(), ctr.ServiceSpec.CaddyConfig())

		if err = g.validator.Validate(ctx, caddyfileCandidate); err != nil {
			g.log.Error("Custom Caddy config for service is invalid, skipping it.",
				"service", ctr.ServiceName(), "err", err)
			continue
		}

		caddyfile = caddyfileCandidate
	}

	return caddyfile, nil
}

func (g *CaddyfileGenerator) generateBaseFromPorts(containers []api.ServiceContainer) (string, error) {
	httpHostUpstreams, httpsHostUpstreams := httpUpstreamsFromContainers(containers)

	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("Caddyfile").Funcs(funcs).Parse(caddyfileTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse Caddyfile template: %w", err)
	}

	data := struct {
		VerifyPath         string
		VerifyResponse     string
		HTTPHostUpstreams  map[string][]string
		HTTPSHostUpstreams map[string][]string
	}{
		VerifyPath:         VerifyPath,
		VerifyResponse:     g.machineID,
		HTTPHostUpstreams:  httpHostUpstreams,
		HTTPSHostUpstreams: httpsHostUpstreams,
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute Caddyfile template: %w", err)
	}

	return buf.String(), nil
}

// httpUpstreamsFromContainers extracts upstreams for HTTP and HTTPS protocols from the published ports of the provided
// service containers. It's expected that all containers are healthy.
func httpUpstreamsFromContainers(containers []api.ServiceContainer) (map[string][]string, map[string][]string) {
	// Maps hostnames to lists of upstreams (container IP:port pairs).
	httpHostUpstreams := make(map[string][]string)
	httpsHostUpstreams := make(map[string][]string)
	for _, ctr := range containers {
		ip := ctr.UncloudNetworkIP()
		if !ip.IsValid() {
			// Container is not connected to the uncloud Docker network (could be host network).
			continue
		}
		log := slog.With("container", ctr.ID)

		ports, err := ctr.ServicePorts()
		if err != nil {
			log.Error("Failed to parse service ports for container.", "err", err)
			continue
		}

		for _, port := range ports {
			if port.Mode != api.PortModeIngress {
				continue
			}

			switch port.Protocol {
			case api.ProtocolHTTP:
				upstream := net.JoinHostPort(ip.String(), strconv.Itoa(int(port.ContainerPort)))
				httpHostUpstreams[port.Hostname] = append(httpHostUpstreams[port.Hostname], upstream)
			case api.ProtocolHTTPS:
				upstream := net.JoinHostPort(ip.String(), strconv.Itoa(int(port.ContainerPort)))
				httpsHostUpstreams[port.Hostname] = append(httpsHostUpstreams[port.Hostname], upstream)
			default:
				// TODO: implement L4 ingress routing for TCP and UDP.
				log.Error("Unsupported protocol for ingress port.", "port", port)
				continue
			}
		}
	}

	return httpHostUpstreams, httpsHostUpstreams
}
