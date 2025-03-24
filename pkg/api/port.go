package api

import (
	"fmt"
	"net/netip"
	"slices"
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
	// Protocol specifies the network protocol.
	Protocol string
	// Mode specifies how the port is published.
	Mode string
}

func (p *PortSpec) Validate() error {
	if p.ContainerPort == 0 {
		return fmt.Errorf("container port must be non-zero")
	}

	switch p.Protocol {
	case "":
		return fmt.Errorf("protocol must be specified")
	case ProtocolHTTP, ProtocolHTTPS, ProtocolTCP, ProtocolUDP:
	default:
		return fmt.Errorf("invalid protocol '%s', supported protocols: '%s', '%s', '%s', '%s'",
			p.Protocol, ProtocolHTTP, ProtocolHTTPS, ProtocolTCP, ProtocolUDP)
	}

	switch p.Mode {
	case "":
		return fmt.Errorf("mode must be specified")
	case PortModeIngress:
		if p.HostIP.IsValid() {
			return fmt.Errorf("host IP cannot be specified in %s mode", PortModeIngress)
		}
		if p.Hostname != "" {
			if p.Protocol != ProtocolHTTP && p.Protocol != ProtocolHTTPS {
				return fmt.Errorf("hostname is only valid with '%s' or '%s' protocols", ProtocolHTTP, ProtocolHTTPS)
			}
			if err := validateHostname(p.Hostname); err != nil {
				return fmt.Errorf("invalid hostname '%s': %w", p.Hostname, err)
			}
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

// String returns the port specification in the -p/--publish flag format.
// Format:
// [hostname:][load_balancer_port:]container_port/protocol for ingress mode (default) or
// [host_ip:]:host_port:container_port/protocol@host for host mode.
func (p *PortSpec) String() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}

	var parts []string

	switch p.Mode {
	case "", PortModeIngress: // [hostname:][load_balancer_port:]container_port/protocol
		if p.Hostname != "" {
			parts = append(parts, p.Hostname)
		}
		if p.PublishedPort != 0 {
			parts = append(parts, fmt.Sprint(p.PublishedPort))
		}
		parts = append(parts, fmt.Sprint(p.ContainerPort))

		return fmt.Sprintf("%s/%s", strings.Join(parts, ":"), p.Protocol), nil
	case PortModeHost: // [host_ip:]:host_port:container_port/protocol@host
		if p.HostIP.IsValid() {
			if p.HostIP.Is6() {
				parts = append(parts, fmt.Sprintf("[%s]", p.HostIP))
			} else {
				parts = append(parts, p.HostIP.String())
			}
		}
		parts = append(parts, fmt.Sprint(p.PublishedPort))
		parts = append(parts, fmt.Sprint(p.ContainerPort))

		return fmt.Sprintf("%s/%s@host", strings.Join(parts, ":"), p.Protocol), nil
	default:
		return "", fmt.Errorf("not implemented for mode: '%s'", p.Mode)
	}
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

		if parts[0] == "" {
			return spec, fmt.Errorf("hostname or published port must be specified, format: " +
				"hostname:container_port or published_port:container_port")
		}
		// Try to parse the first part as port.
		if publishedPort, err := parsePort(parts[0]); err == nil {
			spec.PublishedPort = publishedPort
		} else {
			// It's a hostname.
			if spec.Mode == PortModeHost {
				return spec, fmt.Errorf("hostname cannot be specified in host mode")
			}
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
			// Hostname may be empty.
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

func validateHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("must not be empty")
	}
	if !strings.Contains(hostname, ".") {
		return fmt.Errorf("must be a valid domain name containing at least one dot")
	}
	return nil
}

// PortsEqual returns true if the two port sets are equal. The order of the ports is not important.
func PortsEqual(a, b []PortSpec) bool {
	if len(a) != len(b) {
		return false
	}

	var err error
	aSerialised := make([]string, len(a))
	bSerialised := make([]string, len(b))

	for i := range a {
		aSerialised[i], err = a[i].String()
		if err != nil {
			return false
		}

		bSerialised[i], err = b[i].String()
		if err != nil {
			return false
		}
	}

	slices.Sort(aSerialised)
	slices.Sort(bSerialised)

	return slices.Equal(aSerialised, bSerialised)
}
