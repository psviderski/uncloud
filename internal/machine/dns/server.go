package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/psviderski/uncloud/internal/machine/network"
)

const (
	// InternalDomain is the cluster internal domain for service discovery. All DNS queries ending with this suffix
	// will be resolved using the internal DNS server.
	InternalDomain = "internal."
	// Port is the standard DNS port.
	Port = 53
	// maxConcurrentForwards is the maximum number of concurrent forwarded queries to upstream DNS servers.
	// 1024 is the default used by the Docker internal DNS server.
	maxConcurrentForwards = 1024
	// forwardingTimeout is the timeout for forwarding a DNS query to an upstream server.
	forwardingTimeout = 3 * time.Second
)

// Resolver is an interface for resolving service names to IP addresses.
type Resolver interface {
	// Resolve returns a list of IP addresses of the service containers.
	// An empty list is returned if no service is found.
	Resolve(serviceName string) []netip.Addr
}

// Server is an embedded internal DNS server for service discovery and forwarding external queries
// to upstream DNS servers.
type Server struct {
	listenAddr      netip.Addr
	localSubnet     netip.Prefix
	resolver        Resolver
	resolvedCount   int
	upstreamServers []netip.AddrPort

	udpServer        *dns.Server
	tcpServer        *dns.Server
	inProgressReqs   sync.WaitGroup
	forwardSemaphore chan struct{}
	log              *slog.Logger
}

// NewServer creates a new DNS server with the given configuration.
// If upstreams is nil, nameservers from /etc/resolv.conf will be used. An empty upstreams list means to only resolve
// internal DNS queries and not forward any external queries.
func NewServer(localSubnet netip.Prefix, resolver Resolver, upstreams []netip.AddrPort) (*Server, error) {
	var listenAddr = network.MachineIP(localSubnet)
	if !listenAddr.IsValid() {
		return nil, fmt.Errorf("invalid listen address: %s", listenAddr)
	}
	if resolver == nil {
		return nil, fmt.Errorf("resolver must be provided")
	}

	// Load upstreams from /etc/resolv.conf and set fallback servers only if upstreams is not provided.
	if upstreams == nil {
		if nameservers, err := parseNameserversFromResolvConf(); err != nil {
			slog.Warn("Failed to parse nameservers from /etc/resolv.conf.", "err", err)
		} else {
			for _, ns := range nameservers {
				if ns.Compare(listenAddr) == 0 {
					// Skip this internal DNS server address if it has been configured in /etc/resolv.conf
					// to not create a forwarding loop.
					continue
				}
				upstreams = append(upstreams, netip.AddrPortFrom(ns, Port))
			}
		}

		// Fallback to common public DNS servers if no nameservers were found in /etc/resolv.conf.
		if upstreams == nil {
			upstreams = []netip.AddrPort{
				netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), Port), // Cloudflare DNS
				netip.AddrPortFrom(netip.MustParseAddr("8.8.8.8"), Port), // Google DNS
			}
		}
	}

	return &Server{
		listenAddr:       listenAddr,
		localSubnet:      localSubnet,
		resolvedCount:    0,
		resolver:         resolver,
		upstreamServers:  upstreams,
		forwardSemaphore: make(chan struct{}, maxConcurrentForwards),
		log:              slog.With("component", "dns-server"),
	}, nil
}

// ListenAddr returns the address the DNS server is listening on.
func (s *Server) ListenAddr() netip.Addr {
	return s.listenAddr
}

// Run starts the DNS server listening on both UDP and TCP ports. The server on TCP is not critical so it won't return
// an error if it fails to start. The server will run until the context is canceled or an error occurs.
func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.listenAddr.String(), strconv.Itoa(Port))
	s.udpServer = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleRequest),
	}
	s.tcpServer = &dns.Server{
		Addr:    addr,
		Net:     "tcp",
		Handler: dns.HandlerFunc(s.handleRequest),
	}

	errCh := make(chan error, 1) // Buffer size 1 is for UDP server error only.

	go func() {
		s.log.Info("Starting DNS server on UDP port.", "addr", addr, "upstreams", s.upstreamServers)
		if err := s.udpServer.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("listen and serve on %s/udp: %w", addr, err)
		}
	}()

	go func() {
		s.log.Info("Starting DNS server on TCP port.", "addr", addr, "upstreams", s.upstreamServers)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			// TCP server is not critical, so log the error and continue.
			slog.Warn("Failed to listen and serve DNS server on TCP port. "+
				"TCP server is not critical and will be ignored.", "addr", addr, "err", err)
		}
	}()

	select {
	case err := <-errCh:
		// Stop the servers if the UDP one fails.
		s.stop()
		return err
	case <-ctx.Done():
		s.log.Info("Stopping DNS server.")
		return s.stop()
	}
}

// stop gracefully shuts down the DNS server.
func (s *Server) stop() error {
	var udpErr, tcpErr error

	if s.udpServer != nil {
		udpErr = s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		tcpErr = s.tcpServer.Shutdown()
	}

	// Wait for all in-progress requests to finish.
	s.inProgressReqs.Wait()

	if udpErr != nil {
		return fmt.Errorf("shutdown DNS server (UDP): %w", udpErr)
	}

	if tcpErr != nil {
		return fmt.Errorf("shutdown DNS server (TCP): %w", tcpErr)
	}

	return nil
}

// handleRequest processes a DNS query and returns an appropriate response.
func (s *Server) handleRequest(w dns.ResponseWriter, req *dns.Msg) {
	s.inProgressReqs.Add(1)
	defer s.inProgressReqs.Done()

	if len(req.Question) == 0 {
		resp := new(dns.Msg).SetRcode(req, dns.RcodeFormatError)
		s.reply(w, req, resp)
		return
	}

	// While the original DNS RFCs allow multiple questions, in practice it never works. So handle only the first one.
	q := req.Question[0]
	log := s.log.With("name", q.Name, "type", dns.TypeToString[q.Qtype])
	log.Debug("Received DNS query.")

	if !dns.IsSubDomain(InternalDomain, dns.CanonicalName(q.Name)) {
		log.Debug("Forwarding non-internal DNS query to upstream DNS servers.")

		// Use the same transport for the forwarded request as the original request.
		resp, err := s.forwardRequest(req, w.LocalAddr().Network())
		if err != nil {
			log.Error("Failed to forward DNS query.", "err", err)
			resp = new(dns.Msg).SetRcode(req, dns.RcodeServerFailure)
		}

		s.reply(w, req, resp)
		return
	}

	// Handle the query for the internal domain.
	resp := new(dns.Msg).SetReply(req)
	resp.Authoritative = true
	resp.RecursionAvailable = true

	switch q.Qtype {
	case dns.TypeA:
		records := s.handleAQuery(q.Name)
		if len(records) > 0 {
			log.Debug("Found A records for internal DNS query.", "count", len(records))
			resp.Answer = append(resp.Answer, records...)
		} else {
			log.Debug("No records found for internal DNS query.")
			resp.SetRcode(req, dns.RcodeNameError)
		}
		// TODO: Handle other query types (SRV, TXT, etc.) as needed.
	}

	// Truncate the response if it exceeds the maximum size for the transport protocol.
	maxSize := dns.MinMsgSize
	if w.LocalAddr().Network() == "tcp" {
		maxSize = dns.MaxMsgSize
	} else {
		// Retrieve the UDP buffer size from the EDNS0 record if present.
		if opt := req.IsEdns0(); opt != nil {
			if udpSize := int(opt.UDPSize()); udpSize > maxSize {
				maxSize = udpSize
			}
		}
	}
	resp.Truncate(maxSize)

	s.reply(w, req, resp)
}

func (s *Server) reply(w dns.ResponseWriter, req *dns.Msg, resp *dns.Msg) error {
	if err := w.WriteMsg(resp); err != nil {
		s.log.Error("Failed to write DNS response.", "err", err, "msg", resp)
		// It may fail due to a malformed response message, e.g. exceeding the maximum size. In that case,
		// attempt to send a server failure response instead so the client doesn't have to wait for a timeout.
		if resp.Rcode != dns.RcodeServerFailure {
			resp = new(dns.Msg).SetRcode(req, dns.RcodeServerFailure)
			if err2 := w.WriteMsg(resp); err2 != nil {
				s.log.Error("Failed to write DNS error response.", "err", err2, "msg", resp)
			}
		}
		return err
	}
	return nil
}

// forwardRequest forwards a DNS query to system DNS servers
func (s *Server) forwardRequest(req *dns.Msg, proto string) (*dns.Msg, error) {
	if len(s.upstreamServers) == 0 {
		return nil, errors.New("no upstream DNS servers configured")
	}

	// Apply concurrency control for forwarded queries.
	select {
	case s.forwardSemaphore <- struct{}{}:
		defer func() {
			<-s.forwardSemaphore
		}()
	default:
		return nil, fmt.Errorf("too many concurrent forwarded queries (max: %d)", maxConcurrentForwards)
	}

	// Create DNS client with timeout and same transport as original request
	client := &dns.Client{
		Net:     proto,
		Timeout: forwardingTimeout,
	}

	var lastErr error
	for _, server := range s.upstreamServers {
		resp, _, err := client.Exchange(req, server.String())
		if err == nil {
			return resp, nil
		}
		lastErr = err
		s.log.Debug("Failed to forward DNS query to upstream server.", "server", server, "err", err)
	}

	return nil, lastErr
}

// handleAQuery processes an A query for the internal domain and returns A records for the requested name.
// The internal domain suffix is already stripped from the name. An empty list is returned if no records are found.
func (s *Server) handleAQuery(name string) []dns.RR {
	serviceName, mode := extractModeFromDomain(trimInternalDomain(name))
	ips := s.resolver.Resolve(serviceName)
	if len(ips) == 0 {
		s.log.Debug("Failed to resolve service name.", "service", serviceName)
		return nil
	}
	s.log.Debug("Resolved service name.", "service", serviceName, "ips", ips)

	if len(ips) > 1 {
		// Shuffle the IPs to approximate a round-robin distribution.
		rand.Shuffle(len(ips), func(i, j int) {
			ips[i], ips[j] = ips[j], ips[i]
		})
		// if name was "nearest.{service}", move any machine-local IPs to the top
		// TODO: sort by proximity/latency to the requesting container/machine.
		if mode == "nearest" {
			slices.SortFunc(ips, func(a, b netip.Addr) int {
				aIsLocal := s.localSubnet.Contains(a)
				bIsLocal := s.localSubnet.Contains(b)
				if aIsLocal && !bIsLocal {
					return -1
				} else if bIsLocal && !aIsLocal {
					return 1
				}
				return 0
			})
		}
	}

	// Create A records for each IP.
	records := make([]dns.RR, 0, len(ips))
	for _, ip := range ips {
		records = append(records, &dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				// TODO: should we increate the TTL to some reasonably small value like 5-30 seconds to allow
				//  at least some caching?
				Ttl: 0,
			},
			A: net.ParseIP(ip.String()),
		})
	}
	return records
}

// parseNameserversFromResolvConf parses the nameservers from /etc/resolv.conf.
func parseNameserversFromResolvConf() ([]netip.Addr, error) {
	config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}

	var servers []netip.Addr
	for _, server := range config.Servers {
		if addr, err := netip.ParseAddr(server); err == nil {
			servers = append(servers, addr)
		}
	}

	return servers, nil
}

func trimInternalDomain(name string) string {
	name = dns.CanonicalName(name)
	if !dns.IsSubDomain(InternalDomain, name) {
		return name
	}

	return strings.TrimSuffix(name, "."+InternalDomain)
}

func extractModeFromDomain(name string) (string, string) {
	modes := []string {"nearest"}
	for _, mode := range modes {
		if cut, found := strings.CutPrefix(name, mode + "."); found {
			return cut, mode
		}
	}
	return name, ""
}
