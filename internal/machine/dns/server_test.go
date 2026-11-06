package dns

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testResolver map[string][]netip.Addr

func (r testResolver) Resolve(serviceName string) []netip.Addr {
	return append([]netip.Addr(nil), r[serviceName]...)
}

// startTestServer starts a real UDP server so requests reach the handler only partially unpacked like in production.
func startTestServer(t *testing.T, resolver testResolver) func(req *dns.Msg) *dns.Msg {
	t.Helper()

	server, err := NewServer(
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParsePrefix("10.210.0.0/16"),
		resolver,
		[]netip.AddrPort{},
	)
	require.NoError(t, err)

	stop, addr, err := dnstest.UDPServer("127.0.0.1:0", func(s *dns.Server) {
		s.Handler = dns.HandlerFunc(server.handleRequest)
	})
	require.NoError(t, err)
	t.Cleanup(stop)

	return func(req *dns.Msg) *dns.Msg {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		resp, err := dns.Exchange(ctx, req, "udp", addr)
		require.NoError(t, err)
		return resp
	}
}

func TestHandleRequestInternalA(t *testing.T) {
	t.Parallel()

	ips := func(n int) []netip.Addr {
		addrs := make([]netip.Addr, 0, n)
		for i := 1; i <= n; i++ {
			addrs = append(addrs, netip.MustParseAddr(fmt.Sprintf("10.210.0.%d", i)))
		}
		return addrs
	}

	tests := []struct {
		name          string
		resolver      testResolver
		udpSize       uint16
		wantRcode     uint16
		wantTruncated bool
		wantAnswers   int
	}{
		{
			name:        "resolves to A records",
			resolver:    testResolver{"web": ips(1)},
			wantRcode:   dns.RcodeSuccess,
			wantAnswers: 1,
		},
		{
			name:      "unknown service returns NXDOMAIN",
			resolver:  testResolver{},
			wantRcode: dns.RcodeNameError,
		},
		{
			name:          "too large response is truncated",
			resolver:      testResolver{"web": ips(40)},
			wantRcode:     dns.RcodeSuccess,
			wantTruncated: true,
		},
		{
			name:        "EDNS buffer size allows larger response",
			resolver:    testResolver{"web": ips(40)},
			udpSize:     1232,
			wantRcode:   dns.RcodeSuccess,
			wantAnswers: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exchange := startTestServer(t, tt.resolver)

			req := dns.NewMsg("web.internal.", dns.TypeA)
			req.UDPSize = tt.udpSize
			resp := exchange(req)

			assert.True(t, resp.Authoritative)
			assert.EqualValues(t, tt.wantRcode, resp.Rcode)
			assert.Equal(t, tt.wantTruncated, resp.Truncated)
			assert.Len(t, resp.Answer, tt.wantAnswers)
		})
	}
}

// TestHandleRequestPooledReadBuffers sends alternating small and large queries to verify that handling a query
// never shrinks the server's pooled read buffers. A regression here makes the server silently drop queries that
// are larger than a previously written response.
func TestHandleRequestPooledReadBuffers(t *testing.T) {
	t.Parallel()

	exchange := startTestServer(t, testResolver{"web": {netip.MustParseAddr("10.210.0.2")}})

	longName := strings.Repeat("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.", 6) + "internal."
	for range 5 {
		resp := exchange(dns.NewMsg("web.internal.", dns.TypeA))
		require.EqualValues(t, dns.RcodeSuccess, resp.Rcode)

		resp = exchange(dns.NewMsg(longName, dns.TypeA))
		require.EqualValues(t, dns.RcodeNameError, resp.Rcode)
	}
}
