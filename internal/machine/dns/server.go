package dns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsconf"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/miekg/dns/rdata"
	"github.com/psviderski/uncloud/internal/metrics"
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
	upstreamServers []netip.AddrPort

	udpServer        *dns.Server
	tcpServer        *dns.Server
	udpStarted       chan struct{}
	udpFailed        chan struct{}
	tcpStarted       chan struct{}
	tcpFailed        chan struct{}
	forwardClient    *dns.Client
	inProgressReqs   sync.WaitGroup
	forwardSemaphore chan struct{}
	log              *slog.Logger
}

// NewServer creates a new DNS server with the given configuration.
// If upstreams is nil, nameservers from /etc/resolv.conf will be used. An empty upstreams list means to only resolve
// internal DNS queries and not forward any external queries.
func NewServer(listenAddr netip.Addr, localSubnet netip.Prefix, resolver Resolver, upstreams []netip.AddrPort) (*Server, error) {
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
		listenAddr:      listenAddr,
		localSubnet:     localSubnet,
		resolver:        resolver,
		upstreamServers: upstreams,
		// dns.Client is safe for concurrent use, so share one client across all forwarded queries.
		forwardClient: &dns.Client{
			Transport: &dns.Transport{
				Dialer:       &net.Dialer{Timeout: forwardingTimeout},
				ReadTimeout:  forwardingTimeout,
				WriteTimeout: forwardingTimeout,
			},
		},
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
	s.udpStarted, s.udpFailed = make(chan struct{}), make(chan struct{})
	s.tcpStarted, s.tcpFailed = make(chan struct{}), make(chan struct{})

	s.udpServer = dns.NewServer()
	s.udpServer.Addr = addr
	s.udpServer.Net = "udp"
	s.udpServer.Handler = dns.HandlerFunc(s.handleRequest)
	udpStarted := s.udpStarted
	s.udpServer.NotifyStartedFunc = func(context.Context) {
		close(udpStarted)
	}

	s.tcpServer = dns.NewServer()
	s.tcpServer.Addr = addr
	s.tcpServer.Net = "tcp"
	s.tcpServer.Handler = dns.HandlerFunc(s.handleRequest)
	tcpStarted := s.tcpStarted
	s.tcpServer.NotifyStartedFunc = func(context.Context) {
		close(tcpStarted)
	}

	errCh := make(chan error, 1) // Buffer size 1 is for UDP server error only.

	go func() {
		s.log.Info("Starting DNS server on UDP port.", "addr", addr, "upstreams", s.upstreamServers)
		if err := s.udpServer.ListenAndServe(); err != nil {
			close(s.udpFailed)
			errCh <- fmt.Errorf("listen and serve on %s/udp: %w", addr, err)
		}
	}()

	go func() {
		s.log.Info("Starting DNS server on TCP port.", "addr", addr, "upstreams", s.upstreamServers)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			close(s.tcpFailed)
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
		s.stop()
		return nil
	}
}

// stop gracefully shuts down the DNS server.
func (s *Server) stop() {
	shutdownServer(s.udpServer, s.udpStarted, s.udpFailed)
	shutdownServer(s.tcpServer, s.tcpStarted, s.tcpFailed)

	// Wait for all in-progress requests to finish.
	s.inProgressReqs.Wait()
}

// shutdownServer shuts down srv once the outcome of its startup is known. dns.Server.Shutdown blocks forever
// if the server never started listening, so wait until the server either reports it started or its
// ListenAndServe call returned an error.
func shutdownServer(srv *dns.Server, started, failed <-chan struct{}) {
	if srv == nil {
		return
	}
	select {
	case <-started:
		srv.Shutdown(context.Background())
	case <-failed:
	}
}

// handleRequest processes a DNS query and returns an appropriate response.
func (s *Server) handleRequest(ctx context.Context, w dns.ResponseWriter, req *dns.Msg) {
	s.inProgressReqs.Add(1)
	defer s.inProgressReqs.Done()

	if len(req.Question) == 0 {
		resp := newResponse(req, dns.RcodeFormatError)
		s.reply(w, req, resp)
		return
	}

	// While the original DNS RFCs allow multiple questions, in practice it never works. So handle only the first one.
	qName, qType := dnsutil.Question(req)
	log := s.log.With("name", qName, "type", dnsutil.TypeToString(qType))
	log.Debug("Received DNS query.")

	if !dnsutil.IsBelow(InternalDomain, dnsutil.Canonical(qName)) {
		log.Debug("Forwarding non-internal DNS query to upstream DNS servers.")

		// Use the same transport for the forwarded request as the original request.
		resp, err := s.forwardRequest(ctx, req, w.LocalAddr().Network())
		if err != nil {
			log.Error("Failed to forward DNS query.", "err", err)
			resp = newResponse(req, dns.RcodeServerFailure)
		}
		metrics.DNSQuery.WithLabelValues("false", metrics.Status(err)).Inc()

		s.reply(w, req, resp)
		return
	}

	// Handle the query for the internal domain.
	maxSize := responseMaxSize(w, req)
	resp := newResponse(req, dns.RcodeSuccess)
	resp.Authoritative = true
	resp.RecursionAvailable = true

	switch qType {
	case dns.TypeA:
		records, found := s.handleAQuery(qName)
		if found {
			log.Debug("Found A records for internal DNS query.", "count", len(records))
			resp.Answer = append(resp.Answer, records...)
		} else {
			log.Debug("No records found for internal DNS query.")
			resp.Rcode = dns.RcodeNameError
		}
		// TODO: Handle other query types (SRV, TXT, etc.) as needed.
	}

	truncateResponse(resp, maxSize)
	metrics.DNSQuery.WithLabelValues("true", metrics.Ok).Inc() // NameError is not an error

	s.reply(w, req, resp)
}

func (s *Server) reply(w dns.ResponseWriter, req *dns.Msg, resp *dns.Msg) error {
	if _, err := io.Copy(w, resp); err != nil {
		s.log.Error("Failed to write DNS response.", "err", err, "msg", resp)
		// It may fail due to a malformed response message, e.g. exceeding the maximum size. In that case,
		// attempt to send a server failure response instead so the client doesn't have to wait for a timeout.
		if resp.Rcode != dns.RcodeServerFailure {
			resp = newResponse(req, dns.RcodeServerFailure)
			if _, err2 := io.Copy(w, resp); err2 != nil {
				s.log.Error("Failed to write DNS error response.", "err", err2, "msg", resp)
			}
		}
		return err
	}
	return nil
}

// newResponse creates a fresh response message for req with the given response code. A new message is used
// instead of mutating req in place so the request's pooled Data buffer never ends up in the response.
func newResponse(req *dns.Msg, rcode uint16) *dns.Msg {
	resp := dnsutil.SetReply(new(dns.Msg), req)
	resp.Rcode = rcode
	return resp
}

func responseMaxSize(w dns.ResponseWriter, req *dns.Msg) int {
	if w.LocalAddr().Network() == "tcp" {
		return dns.MaxMsgSize
	}
	if req.UDPSize > dns.MinMsgSize {
		return int(req.UDPSize)
	}
	return dns.MinMsgSize
}

// truncateResponse packs resp and drops answer records until the packed message fits maxSize.
// Pack reuses resp.Data when its capacity is sufficient, so the buffer must not be reset between iterations.
func truncateResponse(resp *dns.Msg, maxSize int) {
	if maxSize < dns.MinMsgSize {
		maxSize = dns.MinMsgSize
	}
	for {
		if err := resp.Pack(); err != nil || len(resp.Data) <= maxSize || len(resp.Answer) == 0 {
			return
		}
		resp.Truncated = true
		resp.Answer = resp.Answer[:len(resp.Answer)-1]
	}
}

// forwardRequest forwards a DNS query to system DNS servers
func (s *Server) forwardRequest(ctx context.Context, req *dns.Msg, proto string) (*dns.Msg, error) {
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

	var lastErr error
	for _, server := range s.upstreamServers {
		// Clone the request data for every attempt: Exchange reads the response into the request's Data
		// buffer, so a failed attempt may leave partial response bytes in it.
		forwardReq := &dns.Msg{
			MsgHeader: req.MsgHeader,
			Question:  req.Question,
			Data:      slices.Clone(req.Data),
		}
		// Data is already packed so UDPSize doesn't change the message on the wire: the upstream server
		// still sees the EDNS buffer size advertised by the original client. It only makes Exchange
		// allocate a read buffer large enough for any possible response.
		forwardReq.UDPSize = dns.MaxMsgSize

		reqCtx, cancel := context.WithTimeout(ctx, forwardingTimeout)
		resp, _, err := s.forwardClient.Exchange(reqCtx, forwardReq, proto, server.String())
		cancel()
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
func (s *Server) handleAQuery(name string) ([]dns.RR, bool) {
	serviceName, mode := extractModeFromDomain(trimInternalDomain(name))
	ips := s.resolver.Resolve(serviceName)
	if len(ips) == 0 {
		s.log.Debug("Failed to resolve service name.", "service", serviceName)
		return nil, false
	}
	s.log.Debug("Resolved service name.", "service", serviceName, "ips", ips)

	if len(ips) > 1 {
		// Shuffle the IPs to approximate round-robin.
		// We want to do this as a baseline for "nearest" mode, as well.
		rand.Shuffle(len(ips), func(i, j int) {
			ips[i], ips[j] = ips[j], ips[i]
		})

		// Default (mode == "") currently behaves the same as round-robin,
		// and nothing additional to do for round-robin (mode == "rr").

		if mode == "nearest" {
			// Sort IPs on local subnet to the top.
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
		// Unmap 4-in-6 mapped addresses so they are recognized and packed as IPv4. Only IPv4 addresses
		// can be returned in A records.
		ip = ip.Unmap()
		if !ip.Is4() {
			continue
		}
		records = append(records, &dns.A{
			Hdr: dns.Header{
				Name:  name,
				Class: dns.ClassINET,
				// TODO: should we increate the TTL to some reasonably small value like 5-30 seconds to allow
				//  at least some caching?
				TTL: 0,
			},
			A: rdata.A{Addr: ip},
		})
	}
	return records, true
}

// parseNameserversFromResolvConf parses the nameservers from /etc/resolv.conf.
func parseNameserversFromResolvConf() ([]netip.Addr, error) {
	config, err := dnsconf.FromFile("/etc/resolv.conf")
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
	name = dnsutil.Canonical(name)
	if !dnsutil.IsBelow(InternalDomain, name) {
		return name
	}

	return strings.TrimSuffix(name, "."+InternalDomain)
}

func extractModeFromDomain(name string) (string, string) {
	modes := []string{"nearest", "rr"}
	for _, mode := range modes {
		if cut, found := strings.CutPrefix(name, mode+"."); found {
			return cut, mode
		}
	}
	return name, ""
}
