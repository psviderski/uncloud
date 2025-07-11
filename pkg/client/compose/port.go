package compose

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

const PortsExtensionKey = "x-ports"

type PortsSource []string

// transformServicesPortsExtension transforms both standard 'ports' and 'x-ports' to PortSpecs.
// Standard 'ports' directive is converted to x-ports string format first, then parsed.
func transformServicesPortsExtension(project *types.Project) (*types.Project, error) {
	return project.WithServicesTransform(func(name string, service types.ServiceConfig) (types.ServiceConfig, error) {
		// Check for mutual exclusivity
		hasStandardPorts := len(service.Ports) > 0
		hasXPorts := service.Extensions[PortsExtensionKey] != nil

		if hasStandardPorts && hasXPorts {
			return service, fmt.Errorf("service %q cannot specify both 'ports' and 'x-ports' directives, use only one", name)
		}

		var portsSource PortsSource
		var err error

		if hasStandardPorts {
			// Convert standard ports to x-ports string format
			portsSource, err = convertStandardPortsToXPorts(service.Ports)
			if err != nil {
				return service, fmt.Errorf("convert standard ports for service %q: %w", name, err)
			}
		} else if hasXPorts {
			// Use existing x-ports
			var ok bool
			portsSource, ok = service.Extensions[PortsExtensionKey].(PortsSource)
			if !ok {
				return service, nil
			}
		} else {
			// No ports specified
			return service, nil
		}

		// Parse the port strings using existing logic
		specs, err := transformPortsExtension(portsSource)
		if err != nil {
			return service, err
		}

		// Ensure extensions map exists
		if service.Extensions == nil {
			service.Extensions = make(types.Extensions)
		}
		service.Extensions[PortsExtensionKey] = specs
		return service, nil
	})
}

func transformPortsExtension(ports PortsSource) ([]api.PortSpec, error) {
	var specs []api.PortSpec
	for _, port := range ports {
		spec, err := api.ParsePortSpec(port)
		if err != nil {
			return specs, fmt.Errorf("parse port %q: %w", port, err)
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// expandPortRange expands a port range like "3000-3005" into individual port configs
func expandPortRange(basePort types.ServicePortConfig) ([]types.ServicePortConfig, error) {
	if !strings.Contains(basePort.Published, "-") {
		// Not a range, return as-is
		return []types.ServicePortConfig{basePort}, nil
	}

	parts := strings.Split(basePort.Published, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port range format %q", basePort.Published)
	}

	startPort, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid start port in range %q: %w", basePort.Published, err)
	}

	endPort, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid end port in range %q: %w", basePort.Published, err)
	}

	if startPort >= endPort {
		return nil, fmt.Errorf("invalid port range %q: start port must be less than end port", basePort.Published)
	}

	// Expand range into individual ports
	var expandedPorts []types.ServicePortConfig
	portDiff := endPort - startPort

	for i := uint64(0); i <= portDiff; i++ {
		expandedPort := basePort // Copy the base configuration

		// Set published port
		publishedPort := startPort + i
		expandedPort.Published = strconv.FormatUint(publishedPort, 10)

		// Set target port (aka container port)
		// If target port range matches published port range, expand it too
		// Otherwise, keep the same target port for all
		if basePort.Target != 0 {
			// For ranges, Docker Compose maps ports 1:1 by default
			expandedPort.Target = basePort.Target + uint32(i)
		}

		expandedPorts = append(expandedPorts, expandedPort)
	}

	return expandedPorts, nil
}

// convertStandardPortsToXPorts converts standard Compose ports to x-ports string format.
func convertStandardPortsToXPorts(ports []types.ServicePortConfig) (PortsSource, error) {
	var xportsStrings []string

	for _, port := range ports {
		// Check if this port uses a range that should be expanded
		expandedPorts, err := expandPortRange(port)
		if err != nil {
			return nil, err
		}

		// Convert each expanded port to x-ports string format
		for _, expandedPort := range expandedPorts {
			xportString, err := convertServicePortToXPortString(expandedPort)
			if err != nil {
				return nil, err
			}
			xportsStrings = append(xportsStrings, xportString)
		}
	}

	return PortsSource(xportsStrings), nil
}

// portStringBuilder provides a professional way to build x-ports format strings
// with comprehensive validation and support for Docker Compose specification.
type portStringBuilder struct {
	hostIP      string
	published   string
	target      uint32
	protocol    string
	mode        string
	name        string
	appProtocol string
}

// newPortStringBuilder creates a new builder from ServicePortConfig with validation
func newPortStringBuilder(port types.ServicePortConfig) (*portStringBuilder, error) {
	if port.Target == 0 {
		return nil, fmt.Errorf("container port (target) is required")
	}

	builder := &portStringBuilder{
		hostIP:      port.HostIP,
		published:   port.Published,
		target:      port.Target,
		protocol:    port.Protocol,
		mode:        port.Mode,
		name:        port.Name,
		appProtocol: port.AppProtocol,
	}

	// Apply defaults according to Docker Compose specification
	if builder.protocol == "" {
		builder.protocol = "tcp"
	}
	if builder.mode == "" {
		builder.mode = "ingress"
	}

	return builder, nil
}

// validateHostIP validates the host IP address format including IPv6 support
func (b *portStringBuilder) validateHostIP() error {
	if b.hostIP == "" {
		return nil
	}

	// Parse the IP address to ensure it's valid
	if _, err := netip.ParseAddr(b.hostIP); err != nil {
		return fmt.Errorf("invalid host IP address %q: %w", b.hostIP, err)
	}

	return nil
}

// validateProtocol validates the protocol against allowed values
func (b *portStringBuilder) validateProtocol() error {
	switch strings.ToLower(b.protocol) {
	case "tcp", "udp", "http", "https":
		return nil
	default:
		return fmt.Errorf("unsupported protocol %q, supported protocols: tcp, udp, http, https", b.protocol)
	}
}

// validateMode validates the port mode
func (b *portStringBuilder) validateMode() error {
	switch b.mode {
	case "ingress", "host":
		return nil
	default:
		return fmt.Errorf("unsupported port mode %q, supported modes: ingress, host", b.mode)
	}
}

// validatePublishedPort validates the published port format and range
func (b *portStringBuilder) validatePublishedPort() error {
	if b.published == "" {
		return nil
	}

	// Port ranges should not reach this function as they are expanded earlier
	if strings.Contains(b.published, "-") {
		return fmt.Errorf("port ranges should be expanded before string building, got: %q", b.published)
	}

	// Single port validation
	port, err := strconv.ParseUint(b.published, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid published port %q: %w", b.published, err)
	}

	if port == 0 || port > 65535 {
		return fmt.Errorf("published port %d out of valid range (1-65535)", port)
	}

	return nil
}

// validatePortRange validates port range format like "3000-3005"
func (b *portStringBuilder) validatePortRange(portRange string) error {
	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return fmt.Errorf("invalid port range format %q, expected format: startPort-endPort", portRange)
	}

	startPort, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
	if err != nil {
		return fmt.Errorf("invalid start port in range %q: %w", portRange, err)
	}

	endPort, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
	if err != nil {
		return fmt.Errorf("invalid end port in range %q: %w", portRange, err)
	}

	if startPort == 0 || startPort > 65535 || endPort == 0 || endPort > 65535 {
		return fmt.Errorf("port range %q contains ports outside valid range (1-65535)", portRange)
	}

	if startPort >= endPort {
		return fmt.Errorf("invalid port range %q: start port must be less than end port", portRange)
	}

	return nil
}

// validate performs comprehensive validation of all fields
func (b *portStringBuilder) validate() error {
	if err := b.validateHostIP(); err != nil {
		return err
	}
	if err := b.validateProtocol(); err != nil {
		return err
	}
	if err := b.validateMode(); err != nil {
		return err
	}
	if err := b.validatePublishedPort(); err != nil {
		return err
	}

	// Additional mode-specific validations
	if b.mode == "host" && b.published == "" {
		return fmt.Errorf("published port is required in host mode")
	}

	return nil
}

// formatHostIP formats the host IP for inclusion in x-ports string
func (b *portStringBuilder) formatHostIP() string {
	if b.hostIP == "" {
		return ""
	}

	// Check if it's an IPv6 address that needs brackets
	if addr, err := netip.ParseAddr(b.hostIP); err == nil && addr.Is6() {
		return fmt.Sprintf("[%s]", b.hostIP)
	}

	return b.hostIP
}

// build constructs the x-ports format string using strings.Builder for optimal performance
func (b *portStringBuilder) build() (string, error) {
	if err := b.validate(); err != nil {
		return "", err
	}

	var builder strings.Builder

	// Add host IP if specified
	if formattedIP := b.formatHostIP(); formattedIP != "" {
		builder.WriteString(formattedIP)
		builder.WriteString(":")
	}

	// Add published port if specified
	if b.published != "" {
		builder.WriteString(b.published)
		builder.WriteString(":")
	}

	// Add container port (required)
	builder.WriteString(strconv.FormatUint(uint64(b.target), 10))

	// Add protocol if not default TCP or in host mode
	if b.protocol != "tcp" || b.mode == "host" {
		builder.WriteString("/")
		builder.WriteString(b.protocol)
	}

	// Add mode if host mode (ingress is default and not included)
	if b.mode == "host" {
		builder.WriteString("@host")
	}

	return builder.String(), nil
}

// convertServicePortToXPortString converts a single ServicePortConfig to x-ports string format.
// This function now uses a professional builder pattern with comprehensive validation
// and full support for Docker Compose specification including IPv6 and port ranges.
func convertServicePortToXPortString(port types.ServicePortConfig) (string, error) {
	builder, err := newPortStringBuilder(port)
	if err != nil {
		return "", err
	}

	return builder.build()
}
