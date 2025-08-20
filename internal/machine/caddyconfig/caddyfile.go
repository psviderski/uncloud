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

// TODO: change upstreams from 'to' to the directive arguments.
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
// The Caddyfile is generated from the service ports of the healthy containers.
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
	// Sort containers by service name and creation time to generate a stable Caddyfile.
	slices.SortStableFunc(containers, func(a, b api.ServiceContainer) int {
		return cmp.Or(
			strings.Compare(a.ServiceName(), b.ServiceName()),
			a.CreatedTime().Compare(b.CreatedTime()),
		)
	})

	caddyfile, err := g.generateBaseFromPorts(containers)
	if err != nil {
		return "", fmt.Errorf("generate base Caddyfile from service ports: %w", err)
	}

	upstreams := serviceUpstreams(containers)

	// Find the 'caddy' service container on this machine. Use the most recent one if multiple exist.
	var caddyCtr *api.ServiceContainer
	for _, cr := range records {
		if cr.MachineID == g.machineID && cr.Container.ServiceName() == CaddyServiceName &&
			(caddyCtr == nil || cr.Container.CreatedTime().Compare(caddyCtr.CreatedTime()) > 0) {
			caddyCtr = &cr.Container
		}
	}

	// If the caddy container is running on this machine and has a custom Caddy config (global),
	// prepend it to the generated Caddyfile and validate it.
	if caddyCtr != nil && caddyCtr.ServiceSpec.CaddyConfig() != "" {
		// Render the custom global Caddy config as a Go template with the upstreams.
		tmplCtx := templateContext{
			Name:      caddyCtr.ServiceName(),
			Upstreams: upstreams,
		}
		renderedConfig, err := renderCaddyfile(tmplCtx, caddyCtr.ServiceSpec.CaddyConfig())
		if err != nil {
			g.log.Error("Failed to render template directives in custom global Caddy config, skipping it.",
				"service", caddyCtr.ServiceName(), "container", caddyCtr.ID, "err", err)
		} else {
			caddyfileCandidate := renderedConfig + "\n\n" + caddyfile

			if err = g.validator.Validate(ctx, caddyfileCandidate); err != nil {
				g.log.Error("Custom global Caddy config is invalid, skipping it.",
					"service", caddyCtr.ServiceName(), "container", caddyCtr.ID, "err", err)
			} else {
				caddyfile = caddyfileCandidate
			}
		}
	}

	// There could be multiple service containers for the same service with different custom Caddy configs, for example,
	// if the service has been partially updated. The most recent container for each service defines the current custom
	// Caddy config for that service.
	latestServiceContainers := make(map[string]api.ServiceContainer, len(containers))
	for _, ctr := range containers {
		if latest, ok := latestServiceContainers[ctr.ServiceName()]; ok {
			if ctr.CreatedTime().Compare(latest.CreatedTime()) > 0 {
				latestServiceContainers[ctr.ServiceName()] = ctr
			}
		} else {
			latestServiceContainers[ctr.ServiceName()] = ctr
		}
	}
	sortedServiceNames := slices.Sorted(maps.Keys(latestServiceContainers))

	// Append a custom Caddy config for each service to the Caddyfile and validate it. If the config for a service
	// is invalid, skip it but continue processing other services to ensure the Caddyfile remains valid.
	for _, serviceName := range sortedServiceNames {
		// Skip the caddy container as we already processed it.
		if serviceName == CaddyServiceName {
			continue
		}

		ctr := latestServiceContainers[serviceName]
		if ctr.ServiceSpec.CaddyConfig() == "" {
			continue
		}

		// Render the template actions in the service's Caddy config.
		tmplCtx := templateContext{
			Name:      serviceName,
			Upstreams: upstreams,
		}
		renderedConfig, err := renderCaddyfile(tmplCtx, ctr.ServiceSpec.CaddyConfig())
		if err != nil {
			g.log.Error("Failed to render template directives in custom Caddy config for service, skipping it.",
				"service", serviceName, "err", err)
			continue
		}

		caddyfileCandidate := fmt.Sprintf("%s\n# Service: %s\n%s\n", caddyfile, serviceName, renderedConfig)
		if err = g.validator.Validate(ctx, caddyfileCandidate); err != nil {
			g.log.Error("Custom Caddy config for service is invalid, skipping it.",
				"service", serviceName, "err", err)
		} else {
			caddyfile = caddyfileCandidate
		}
	}

	return caddyfile, nil
}

func (g *CaddyfileGenerator) generateBaseFromPorts(containers []api.ServiceContainer) (string, error) {
	httpHostUpstreams, httpsHostUpstreams := httpUpstreamsFromPorts(containers)

	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("Caddyfile").Funcs(funcs).Parse(caddyfileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse Caddyfile template: %w", err)
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
		return "", fmt.Errorf("execute Caddyfile template: %w", err)
	}

	return buf.String(), nil
}

// httpUpstreamsFromPorts extracts upstreams for HTTP and HTTPS protocols from the published ports of the provided
// service containers. It's expected that all containers are healthy.
func httpUpstreamsFromPorts(containers []api.ServiceContainer) (map[string][]string, map[string][]string) {
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

// serviceUpstreams creates a map of service names to their container IPs.
// Only includes containers connected to the uncloud Docker network.
func serviceUpstreams(containers []api.ServiceContainer) map[string][]string {
	upstreams := make(map[string][]string)
	for _, ctr := range containers {
		ip := ctr.UncloudNetworkIP()
		if !ip.IsValid() {
			// Container is not connected to the uncloud Docker network (could be host network).
			continue
		}

		serviceName := ctr.ServiceName()
		upstreams[serviceName] = append(upstreams[serviceName], ip.String())
	}

	return upstreams
}

// renderCaddyfile renders a Caddyfile template with the upstreams function and data.
func renderCaddyfile(tmplCtx templateContext, caddyfile string) (string, error) {
	funcs := template.FuncMap{
		"upstreams": upstreamsTemplateFn(tmplCtx),
	}

	tmpl, err := template.New("Caddyfile").Funcs(funcs).Parse(caddyfile)
	if err != nil {
		return "", fmt.Errorf("parse config as Go template: %w", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, tmplCtx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
