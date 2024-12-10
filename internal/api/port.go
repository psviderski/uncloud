package api

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

const (
	PortModeIngress = "ingress"
	PortModeHost    = "host"

	ProtocolHTTP  = "http"
	ProtocolHTTPS = "https"
	ProtocolTCP   = "tcp"
	ProtocolUDP   = "udp"
)

type PortSpec struct {
	// Hostname specifies the DNS name that will route to this service. Only valid in ingress mode.
	Hostname string
	// HostIP is the host IP to bind the PublishedPort to. Only valid in host mode.
	HostIP netip.Addr
	// PublishedPort is the port number exposed outside the container.
	// In ingress mode, this is the load balancer port. In host mode, this is the port bound on the host.
	PublishedPort uint16
	// ContainerPort is the port inside the container that the service listens on.
	ContainerPort uint16
	// Protocol specifies the network protocol. Default is ProtocolHTTPS if Hostname is set, ProtocolTCP otherwise.
	Protocol string
	// Mode specifies how the port is published. Default is PortModeIngress.
	Mode string
}

func (p *PortSpec) Validate() error {
	if p.ContainerPort == 0 {
		return fmt.Errorf("container port must be non-zero")
	}

	switch p.Protocol {
	case ProtocolHTTP, ProtocolHTTPS, ProtocolTCP, ProtocolUDP:
	default:
		return fmt.Errorf("invalid protocol: '%s'", p.Protocol)
	}

	switch p.Mode {
	case PortModeIngress:
		if p.HostIP.IsValid() {
			return fmt.Errorf("host IP cannot be specified in %s mode", PortModeIngress)
		}
		if p.Hostname != "" && p.Protocol != ProtocolHTTP && p.Protocol != ProtocolHTTPS {
			return fmt.Errorf("hostname is only valid with '%s' or '%s' protocols", ProtocolHTTP, ProtocolHTTPS)
		}
		if p.Hostname == "" && (p.Protocol == ProtocolHTTP || p.Protocol == ProtocolHTTPS) {
			return fmt.Errorf("hostname is required with '%s' or '%s' protocols", ProtocolHTTP, ProtocolHTTPS)
		}
	case PortModeHost:
		if p.PublishedPort == 0 {
			return fmt.Errorf("published port is required in %s mode", PortModeHost)
		}
		if p.Protocol != ProtocolTCP && p.Protocol != ProtocolUDP {
			return fmt.Errorf("unsupported protocol '%s' in %s mode, only '%s' and '%s' are supported",
				p.Protocol, PortModeHost, ProtocolTCP, ProtocolUDP)
		}
		if p.Hostname != "" {
			return fmt.Errorf("hostname cannot be specified in %s mode", PortModeHost)
		}
	default:
		return fmt.Errorf("invalid mode: '%s'", p.Mode)
	}

	return nil
}

func ParsePortSpec(port string) (PortSpec, error) {
	spec := PortSpec{
		Protocol: ProtocolTCP,     // Default protocol.
		Mode:     PortModeIngress, // Default mode.
	}

	// Split off mode first.
	parts := strings.Split(port, "@")
	if len(parts) > 2 {
		return spec, fmt.Errorf("too many '@' symbols")
	}
	if len(parts) == 2 {
		if parts[1] != PortModeHost {
			return spec, fmt.Errorf("invalid mode: '%s', only 'host' mode is supported", parts[1])
		}
		spec.Mode = PortModeHost
	}
	port = parts[0]

	// Parse protocol.
	parts = strings.Split(port, "/")
	if len(parts) > 2 {
		return spec, fmt.Errorf("too many '/' symbols")
	}
	specifiedProtocol := ""
	if len(parts) == 2 {
		protocol := parts[1]
		switch protocol {
		case ProtocolHTTP, ProtocolHTTPS, ProtocolTCP, ProtocolUDP:
			spec.Protocol = protocol
			specifiedProtocol = protocol
		default:
			return spec, fmt.Errorf("unsupported protocol: '%s'", protocol)
		}
	}
	port = parts[0]

	// Parse hostname/host IP and ports.
	parts = splitPortParts(port)
	var err error

	switch len(parts) {
	case 1: // Just container port.
		if spec.ContainerPort, err = parsePort(parts[0]); err != nil {
			return spec, fmt.Errorf("invalid container port '%s': %w", parts[0], err)
		}

	case 2: // hostname:container_port or [load_balancer_port|host_port]:container_port
		if spec.ContainerPort, err = parsePort(parts[1]); err != nil {
			return spec, fmt.Errorf("invalid container port '%s': %w", parts[1], err)
		}

		// Try to parse the first part as port.
		if publishedPort, err := parsePort(parts[0]); err == nil {
			spec.PublishedPort = publishedPort
		} else {
			// It's a hostname.
			if spec.Mode == PortModeHost {
				return spec, fmt.Errorf("hostname cannot be specified in host mode")
			}
			if parts[0] == "" {
				return spec, fmt.Errorf("hostname must not be empty")
			}
			// TODO: validate hostname?
			spec.Hostname = parts[0]
		}

	case 3: // hostname:load_balancer_port:container_port or host_ip:host_port:container_port
		if spec.ContainerPort, err = parsePort(parts[2]); err != nil {
			return spec, fmt.Errorf("invalid container port '%s': %w", parts[2], err)
		}
		if spec.PublishedPort, err = parsePort(parts[1]); err != nil {
			return spec, fmt.Errorf("invalid published port '%s': %w", parts[1], err)
		}

		if spec.Mode == PortModeHost {
			// In host mode, the first part must be IP.
			ip := parts[0]
			// Strip brackets from IPv6 address if present.
			if strings.Contains(ip, ":") {
				if !strings.HasPrefix(ip, "[") {
					return spec, fmt.Errorf(
						"invalid host IP '%s': IPv6 address must be enclosed in square brackets", ip)
				}
				if !strings.HasSuffix(ip, "]") {
					return spec, fmt.Errorf("invalid host IP '%s': missing closing bracket", ip)
				}
				ip = ip[1 : len(ip)-1]
			}

			if spec.HostIP, err = netip.ParseAddr(ip); err != nil {
				return spec, fmt.Errorf("invalid host IP '%s': %w", parts[0], err)
			}
		} else {
			if parts[0] == "" {
				return spec, fmt.Errorf("hostname must not be empty")
			}
			// TODO: validate hostname?
			spec.Hostname = parts[0]
		}

	default:
		return spec, fmt.Errorf("unexpected number of parts in port spec: %d", len(parts))
	}

	if spec.Hostname != "" {
		if specifiedProtocol == "" {
			spec.Protocol = ProtocolHTTPS
		} else if specifiedProtocol != ProtocolHTTP && specifiedProtocol != ProtocolHTTPS {
			return spec, fmt.Errorf("hostname is only valid with '%s' or '%s' protocols, specified: '%s'",
				ProtocolHTTP, ProtocolHTTPS, specifiedProtocol)
		}
	}

	return spec, spec.Validate()
}

// splitPortParts splits a port specification [hostname|host_ip:][published_port:]container_port into its parts.
func splitPortParts(port string) []string {
	parts := strings.Split(port, ":")
	n := len(parts)
	if n > 3 {
		// Host IP may contain colons if it's IPv6, so we need to join the first n-2 parts.
		return append([]string{strings.Join(parts[:n-2], ":")}, parts[n-2:]...)
	}
	return parts
}

func parsePort(s string) (uint16, error) {
	port, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(port), nil
}
