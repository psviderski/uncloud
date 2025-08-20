package caddyconfig

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// templateContext holds the data available to Caddyfile templates.
type templateContext struct {
	// Name is the current service name.
	Name string
	// Upstreams maps service names to their container IPs.
	Upstreams map[string][]string
}

// upstreamsTemplateFn returns a template function that generates a space separated string of upstreams for the service.
// It optionally accepts a service name and a port number: {{upstreams [service-name] [port]}}.
func upstreamsTemplateFn(tmplCtx templateContext) func(args ...any) (string, error) {
	return func(args ...any) (string, error) {
		var serviceName string
		var port int

		// Parse arguments.
		switch len(args) {
		case 0:
			// Current service, default port.
			serviceName = tmplCtx.Name
		case 1:
			// Either port (int) for current service or service name (string).
			switch arg := args[0].(type) {
			case int:
				serviceName = tmplCtx.Name
				port = arg
			case string:
				serviceName = arg
				port = 0
			default:
				return "", fmt.Errorf("upstreams function: invalid argument type: %T", arg)
			}
		case 2:
			// Service name and port.
			name, ok := args[0].(string)
			if !ok {
				return "", fmt.Errorf("upstreams function: first argument must be service name (string)")
			}
			serviceName = name

			p, ok := args[1].(int)
			if !ok {
				return "", fmt.Errorf("upstreams function: second argument must be port (int)")
			}
			port = p
		default:
			return "", fmt.Errorf("upstreams function: too many arguments; expected 0-2, got %d", len(args))
		}

		ips, ok := tmplCtx.Upstreams[serviceName]
		if !ok || len(ips) == 0 {
			// No upstreams available.
			return "", nil
		}

		// Build the space separated upstreams string.
		var upstreams []string
		for _, ip := range ips {
			if port > 0 {
				upstreams = append(upstreams, net.JoinHostPort(ip, strconv.Itoa(port)))
			} else {
				upstreams = append(upstreams, ip)
			}
		}

		return strings.Join(upstreams, " "), nil
	}
}
