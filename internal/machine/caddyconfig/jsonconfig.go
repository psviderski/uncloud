package caddyconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/psviderski/uncloud/pkg/api"
)

func GenerateJSONConfig(containers []api.ServiceContainer, verifyResponse string) (*caddy.Config, error) {
	httpHostUpstreams, httpsHostUpstreams := httpUpstreamsFromPorts(containers)

	var warnings []caddyconfig.Warning
	servers := make(map[string]*caddyhttp.Server)
	servers["http"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPPort)},
		// All http requests to this server are logged to the default logger.
		Logs: &caddyhttp.ServerLogConfig{},
		Routes: append(
			hostUpstreamsToRoutes(httpHostUpstreams, &warnings),
			// Add a route to respond with a static verification response at the /.uncloud-verify path.
			verificationRoute(verifyResponse, &warnings),
		),
	}
	servers["https"] = &caddyhttp.Server{
		Listen: []string{fmt.Sprintf(":%d", caddyhttp.DefaultHTTPSPort)},
		// All https requests to this server are logged to the default logger.
		Logs:   &caddyhttp.ServerLogConfig{},
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
			HealthChecks: &reverseproxy.HealthChecks{
				// Enable passive health checks to automatically detect unhealthy upstreams.
				Passive: &reverseproxy.PassiveHealthChecks{
					FailDuration: caddy.Duration(30 * time.Second),
				},
			},
			LoadBalancing: &reverseproxy.LoadBalancing{
				// Retry failed requests to skip over temporarily unavailable upstreams.
				Retries: 3,
			},
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
