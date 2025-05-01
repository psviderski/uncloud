package caddyfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"slices"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/psviderski/uncloud/pkg/api"
)

func GenerateConfig(containers []api.ServiceContainer, verifyResponse string) (*caddy.Config, error) {
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

	var warnings []caddyconfig.Warning
	servers := make(map[string]*caddyhttp.Server)
	servers["http"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPPort)},
		Routes: append(
			hostUpstreamsToRoutes(httpHostUpstreams, &warnings),
			// Add a route to respond with a static verification response at the /.uncloud-verify path.
			verificationRoute(verifyResponse, &warnings),
		),
	}
	servers["https"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPSPort)},
		Routes: hostUpstreamsToRoutes(httpsHostUpstreams, &warnings),
	}

	httpApp := caddyhttp.App{
		Servers: servers,
	}
	config := &caddy.Config{
		AppsRaw: caddy.ModuleMap{
			"http": caddyconfig.JSON(httpApp, &warnings),
		},
	}

	var err error
	if len(warnings) > 0 {
		// warnings only contains errors from JSON marshaling, which are highly unlikely with correct code.
		for _, w := range warnings {
			err = errors.Join(err, errors.New(w.Message))
		}
		return nil, fmt.Errorf("marshal Caddy configuration: %w", err)
	}

	return config, nil
}

// hostUpstreamsToRoutes converts a map of hostnames to upstreams to a list of Caddy routes.
func hostUpstreamsToRoutes(hostUpstreams map[string][]string, warnings *[]caddyconfig.Warning) []caddyhttp.Route {
	// Sort hostnames for deterministic output.
	hostnames := slices.Collect(maps.Keys(hostUpstreams))
	slices.Sort(hostnames)

	routes := make([]caddyhttp.Route, 0, len(hostUpstreams))
	for _, hostname := range hostnames {
		upstreams := hostUpstreams[hostname]
		upstreamPool := make([]*reverseproxy.Upstream, len(upstreams))
		for i, upstream := range upstreams {
			upstreamPool[i] = &reverseproxy.Upstream{
				Dial: upstream,
			}
		}
		handler := &reverseproxy.Handler{
			Upstreams: upstreamPool,
		}

		routes = append(routes, caddyhttp.Route{
			MatcherSetsRaw: caddyhttp.RawMatcherSets{
				{
					"host": caddyconfig.JSON(caddyhttp.MatchHost{hostname}, warnings),
				},
			},
			HandlersRaw: []json.RawMessage{
				caddyconfig.JSONModuleObject(handler, "handler", "reverse_proxy", warnings),
			},
		})
	}
	return routes
}

// verificationRoute returns a Caddy route that responds with the given static response at the /.uncloud-verify path.
func verificationRoute(response string, warnings *[]caddyconfig.Warning) caddyhttp.Route {
	// Return the following route:
	// {
	//   "match": [
	//     {
	//       "path": [
	//         "/.uncloud-verify"
	//       ]
	//     }
	//   ],
	//   "handle": [
	//     {
	//       "handler": "static_response",
	//       "body": "<response>",
	//       "status_code": 200
	//     }
	//   ]
	// }

	staticResponse := caddyhttp.StaticResponse{
		StatusCode: caddyhttp.WeakString(strconv.Itoa(http.StatusOK)),
		Body:       response,
	}

	return caddyhttp.Route{
		MatcherSetsRaw: caddyhttp.RawMatcherSets{
			{
				"path": caddyconfig.JSON(caddyhttp.MatchPath{VerifyPath}, warnings),
			},
		},
		HandlersRaw: []json.RawMessage{
			caddyconfig.JSONModuleObject(staticResponse, "handler", "static_response", warnings),
		},
	}
}
