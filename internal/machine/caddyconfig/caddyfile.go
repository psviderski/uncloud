package caddyconfig

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"text/template"

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

func GenerateCaddyfile(containers []api.ServiceContainer, verifyResponse string) (string, error) {
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
		VerifyResponse:     verifyResponse,
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
// service containers.
func httpUpstreamsFromContainers(containers []api.ServiceContainer) (map[string][]string, map[string][]string) {
	// Maps hostnames to lists of upstreams (container IP:port pairs).
	httpHostUpstreams := make(map[string][]string)
	httpsHostUpstreams := make(map[string][]string)
	for _, ctr := range containers {
		if !ctr.Healthy() {
			continue
		}

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
