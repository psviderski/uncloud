package cli

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/spf13/cobra"
)

// ExpandCommaSeparatedValues takes a slice of strings and expands any comma-separated values into individual elements.
func ExpandCommaSeparatedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	var expanded []string
	for _, value := range values {
		for v := range strings.SplitSeq(value, ",") {
			if v = strings.TrimSpace(v); v != "" {
				expanded = append(expanded, v)
			}
		}
	}

	return expanded
}

// BindEnvToFlag assigns the value of an environment variable to the given command flag if the flag has not been set.
func BindEnvToFlag(cmd *cobra.Command, flagName, envVar string) {
	if value := os.Getenv(envVar); value != "" && !cmd.Flags().Changed(flagName) {
		if err := cmd.Flags().Set(flagName, value); err != nil {
			log.Fatalf("Failed to bind environment variable '%s' to flag '%s': %v", envVar, flagName, err)
		}
	}
}

// ParseWireGuardEndpoints parses a list of endpoint strings into a list of IPPort protobuf messages. Each value can
// be an IP address, IP:PORT, IPv6, or [IPv6]:PORT. If the port is omitted, the default WireGuard port is used.
func ParseWireGuardEndpoints(values []string) ([]*pb.IPPort, error) {
	endpoints := make([]*pb.IPPort, 0, len(values))
	for _, v := range values {
		ap, err := netip.ParseAddrPort(v)
		if err != nil {
			// Try parsing as a bare IP address and use the default WireGuard port.
			addr, addrErr := netip.ParseAddr(v)
			if addrErr != nil {
				return nil, fmt.Errorf("invalid endpoint '%s': must be IP, IPv6, IP:PORT, or [IPv6]:PORT", v)
			}
			ap = netip.AddrPortFrom(addr, network.WireGuardPort)
		}
		endpoints = append(endpoints, pb.NewIPPort(ap))
	}
	return endpoints, nil
}
