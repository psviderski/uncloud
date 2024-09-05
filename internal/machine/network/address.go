package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ListRoutableIPs returns a list of routable unicast IP addresses.
func ListRoutableIPs() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}

	var routable []netip.Addr
	for _, iface := range interfaces {
		if iface.Name == WireGuardInterfaceName || strings.HasPrefix(iface.Name, "docker") {
			// Skip the Uncloud WireGuard and Docker interfaces.
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
			// Skip interfaces:
			//  * Not administratively UP.
			//  * The operational status is not RUNNING. This is the closest equivalent to checking for NO-CARRIER.
			//  * Loopback.
			continue
		}
		// TODO: check for link/ether ifaces?

		addrs, aErr := iface.Addrs()
		if aErr != nil {
			return nil, fmt.Errorf("list unicast addresses for interface %q: %w", iface.Name, err)
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			// Includes IPv4 private address space and local IPv6 unicast address space.
			if ipNet.IP.IsGlobalUnicast() {
				ip, pErr := netip.ParseAddr(ipNet.IP.String())
				if pErr != nil {
					return nil, fmt.Errorf("parse IP address %q: %w", ipNet.IP, err)
				}
				routable = append(routable, ip)
			}
		}
	}
	return routable, nil
}

func GetPublicIP() (netip.Addr, error) {
	services := []struct {
		URL    string
		Parser func([]byte) (netip.Addr, error)
	}{
		{"https://api.ipify.org", parsePlaintextIP},
		{"https://ipinfo.io/ip", parsePlaintextIP},
		{"http://ip-api.com/line/?fields=query", parsePlaintextIP},
	}

	for _, service := range services {
		if ip, err := queryIP(service.URL, service.Parser); err == nil {
			return ip, nil
		}
	}

	return netip.Addr{}, fmt.Errorf("failed to get public IP from all services")
}

func queryIP(service string, parser func([]byte) (netip.Addr, error)) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, service, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read response body: %w", err)
	}
	return parser(body)
}

func parsePlaintextIP(data []byte) (netip.Addr, error) {
	return netip.ParseAddr(string(data))
}
