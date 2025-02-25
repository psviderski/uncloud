package caddyfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"uncloud/internal/api"
	"uncloud/internal/fs"
	"uncloud/internal/machine/docker"
	"uncloud/internal/machine/store"
)

const (
	CaddyGroup = "uncloud"
	VerifyPath = "/.uncloud-verify"
)

// Controller monitors container changes in the cluster store and generates a configuration file for Caddy reverse
// proxy. The generated Caddyfile allows Caddy to route external traffic to service containers across the internal
// network.
type Controller struct {
	store          *store.Store
	path           string
	verifyResponse string
}

func NewController(store *store.Store, path string, verifyResponse string) (*Controller, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create parent directory for Caddy configuration '%s': %w", dir, err)
	}
	if err := fs.Chown(dir, "", CaddyGroup); err != nil {
		return nil, fmt.Errorf("change owner of parent directory for Caddy configuration '%s': %w", dir, err)
	}

	return &Controller{
		store:          store,
		path:           path,
		verifyResponse: verifyResponse,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	containerRecords, changes, err := c.store.SubscribeContainers(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to container changes: %w", err)
	}
	slog.Info("Subscribed to container changes in the cluster to generate Caddy configuration.")

	containers, err := c.filterAvailableContainers(containerRecords)
	if err != nil {
		return fmt.Errorf("filter available containers: %w", err)
	}
	if err = c.generateConfig(containers); err != nil {
		return fmt.Errorf("generate Caddy configuration: %w", err)
	}

	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return fmt.Errorf("containers subscription failed")
			}
			slog.Debug("Cluster containers changed, updating Caddy configuration.")

			containerRecords, err = c.store.ListContainers(ctx, store.ListOptions{})
			if err != nil {
				slog.Error("Failed to list containers.", "err", err)
				continue
			}
			containers, err = c.filterAvailableContainers(containerRecords)
			if err != nil {
				slog.Error("Failed to filter available containers.", "err", err)
				continue
			}
			if err = c.generateConfig(containers); err != nil {
				slog.Error("Failed to generate Caddy configuration.", "err", err)
			}

			slog.Debug("Updated Caddy configuration.", "path", c.path)
		case <-ctx.Done():
			return nil
		}
	}
}

// filterAvailableContainers filters out containers from this machine that are likely unavailable. The availability
// is determined by the cluster membership state of the machine that the container is running on.
// TODO: implement machine membership check using Corrossion Admin client.
func (c *Controller) filterAvailableContainers(containerRecords []store.ContainerRecord) ([]api.Container, error) {
	containers := make([]api.Container, len(containerRecords))
	for i, cr := range containerRecords {
		containers[i] = cr.Container
	}
	return containers, nil
}

func (c *Controller) generateConfig(containers []api.Container) error {
	// Maps hostnames to lists of upstreams (container IP:port pairs).
	httpHostUpstreams := make(map[string][]string)
	httpsHostUpstreams := make(map[string][]string)
	for _, ctr := range containers {
		logger := slog.With("container", ctr.ID)
		network, ok := ctr.NetworkSettings.Networks[docker.NetworkName]
		if !ok {
			// Container is not connected to the uncloud Docker network (could be host network).
			continue
		}
		if network.IPAddress == "" {
			logger.Error("Container has no IPv4 address.")
			continue
		}

		ports, err := ctr.ServicePorts()
		if err != nil {
			logger.Error("Failed to parse service ports for container.", "err", err)
			continue
		}

		for _, port := range ports {
			switch port.Protocol {
			case api.ProtocolHTTP:
				upstream := net.JoinHostPort(network.IPAddress, strconv.Itoa(int(port.ContainerPort)))
				httpHostUpstreams[port.Hostname] = append(httpHostUpstreams[port.Hostname], upstream)
			case api.ProtocolHTTPS:
				upstream := net.JoinHostPort(network.IPAddress, strconv.Itoa(int(port.ContainerPort)))
				httpsHostUpstreams[port.Hostname] = append(httpHostUpstreams[port.Hostname], upstream)
			default:
				if port.Mode == api.PortModeIngress {
					// TODO: implement L4 ingress routing for TCP and UDP.
					logger.Error("Unsupported protocol for ingress port.", "port", port)
					continue
				}
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
			verificationRoute(c.verifyResponse, &warnings),
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
		return fmt.Errorf("marshal Caddy configuration: %w", err)
	}

	configBytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal Caddy configuration: %w", err)
	}

	if err = os.WriteFile(c.path, configBytes, 0640); err != nil {
		return fmt.Errorf("write Caddy configuration to file '%s': %w", c.path, err)
	}
	if err = fs.Chown(c.path, "", CaddyGroup); err != nil {
		return fmt.Errorf("change owner of Caddy configuration file '%s': %w", c.path, err)
	}

	return nil
}

// hostUpstreamsToRoutes converts a map of hostnames to upstreams to a list of Caddy routes.
func hostUpstreamsToRoutes(hostUpstreams map[string][]string, warnings *[]caddyconfig.Warning) []caddyhttp.Route {
	routes := make([]caddyhttp.Route, 0, len(hostUpstreams))
	for hostname, upstreams := range hostUpstreams {
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
