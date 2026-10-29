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

func newTestServer(t *testing.T, resolver testResolver) *Server {
	t.Helper()

	server, err := NewServer(
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParsePrefix("10.210.0.0/16"),
		resolver,
		[]netip.AddrPort{},
	)
	require.NoError(t, err)
	return server
}

func TestHandleRequestInternalA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		resolver    testResolver
		wantRcode   uint16
		wantAnswers []netip.Addr
	}{
		{
			name:        "resolves to A records",
			resolver:    testResolver{"web": {netip.MustParseAddr("10.210.0.2")}},
			wantRcode:   dns.RcodeSuccess,
			wantAnswers: []netip.Addr{netip.MustParseAddr("10.210.0.2")},
		},
		{
			name:        "unmaps 4-in-6 mapped addresses",
			resolver:    testResolver{"web": {netip.MustParseAddr("::ffff:10.210.0.3")}},
			wantRcode:   dns.RcodeSuccess,
			wantAnswers: []netip.Addr{netip.MustParseAddr("10.210.0.3")},
		},
		{
			// A service with only IPv6 addresses must return an empty answer (NODATA), not NXDOMAIN.
			name:      "IPv6-only service returns no answers",
			resolver:  testResolver{"web": {netip.MustParseAddr("fd00::2")}},
			wantRcode: dns.RcodeSuccess,
		},
		{
			name:      "unknown service returns NXDOMAIN",
			resolver:  testResolver{},
			wantRcode: dns.RcodeNameError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newTestServer(t, tt.resolver)

			req := dns.NewMsg("web.internal.", dns.TypeA)
			require.NoError(t, req.Pack())

			rec := dnstest.NewTestRecorder()
			server.handleRequest(context.Background(), rec, req)

			require.NotNil(t, rec.Msg)
			require.NoError(t, rec.Msg.Unpack())
			assert.True(t, rec.Msg.Response)
			assert.True(t, rec.Msg.Authoritative)
			assert.EqualValues(t, tt.wantRcode, rec.Msg.Rcode)

			var answers []netip.Addr
			for _, rr := range rec.Msg.Answer {
				a, ok := rr.(*dns.A)
				require.True(t, ok, "expected A record, got %T", rr)
				answers = append(answers, a.A.Addr)
			}
			assert.Equal(t, tt.wantAnswers, answers)
		})
	}
}

func TestHandleRequestNearestPrefersLocalMappedAddresses(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, testResolver{"web": {
		netip.MustParseAddr("10.211.0.2"),
		netip.MustParseAddr("10.211.0.3"),
		netip.MustParseAddr("::ffff:10.210.0.4"),
	}})

	req := dns.NewMsg("nearest.web.internal.", dns.TypeA)
	require.NoError(t, req.Pack())

	rec := dnstest.NewTestRecorder()
	server.handleRequest(context.Background(), rec, req)

	require.NotNil(t, rec.Msg)
	require.NoError(t, rec.Msg.Unpack())
	require.Len(t, rec.Msg.Answer, 3)
	assert.Equal(t, netip.MustParseAddr("10.210.0.4"), rec.Msg.Answer[0].(*dns.A).A.Addr)
}

// TestHandleRequestUDPResponseSize queries a real UDP server, so the request reaches the handler only partially
// unpacked like in production.
func TestHandleRequestUDPResponseSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ipCount       int
		udpSize       uint16
		wantTruncated bool
		wantAnswers   int // -1 means any non-zero number of answers.
	}{
		{
			// 25 answers don't fit into 512 bytes uncompressed but fit with name compression.
			name:        "compressed response fits into 512 bytes",
			ipCount:     25,
			wantAnswers: 25,
		},
		{
			name:          "too large response keeps answers that fit",
			ipCount:       80,
			wantTruncated: true,
			wantAnswers:   -1,
		},
		{
			name:        "EDNS buffer size allows larger response",
			ipCount:     60,
			udpSize:     1232,
			wantAnswers: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ips := make([]netip.Addr, 0, tt.ipCount)
			for i := 1; i <= tt.ipCount; i++ {
				ips = append(ips, netip.MustParseAddr(fmt.Sprintf("10.210.0.%d", i)))
			}
			server := newTestServer(t, testResolver{"web": ips})

			stop, addr, err := dnstest.UDPServer("127.0.0.1:0", func(s *dns.Server) {
				s.Handler = dns.HandlerFunc(server.handleRequest)
			})
			require.NoError(t, err)
			defer stop()

			req := dns.NewMsg("web.internal.", dns.TypeA)
			req.UDPSize = tt.udpSize
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			resp, err := dns.Exchange(ctx, req, "udp", addr)
			require.NoError(t, err)

			assert.Equal(t, tt.wantTruncated, resp.Truncated)
			assert.LessOrEqual(t, len(resp.Data), max(int(tt.udpSize), dns.MinMsgSize))
			if tt.wantAnswers == -1 {
				assert.NotEmpty(t, resp.Answer)
			} else {
				assert.Len(t, resp.Answer, tt.wantAnswers)
			}
		})
	}
}

// TestHandleRequestPooledReadBuffers sends alternating small and large queries through a real UDP server
// to verify that handling a query never shrinks the server's pooled read buffers. A regression here makes
// the server silently drop queries that are larger than a previously written response.
func TestHandleRequestPooledReadBuffers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, testResolver{"web": {netip.MustParseAddr("10.210.0.2")}})

	stop, addr, err := dnstest.UDPServer("127.0.0.1:0", func(s *dns.Server) {
		s.Handler = dns.HandlerFunc(server.handleRequest)
	})
	require.NoError(t, err)
	defer stop()

	exchange := func(name string) *dns.Msg {
		t.Helper()

		req := dns.NewMsg(name, dns.TypeA)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		resp, err := dns.Exchange(ctx, req, "udp", addr)
		require.NoError(t, err)
		return resp
	}

	longName := strings.Repeat("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.", 6) + "internal."
	for range 5 {
		resp := exchange("web.internal.")
		require.EqualValues(t, dns.RcodeSuccess, resp.Rcode)

		// A larger query must still be read correctly after the previous response went through
		// the server's buffer pool.
		resp = exchange(longName)
		require.EqualValues(t, dns.RcodeNameError, resp.Rcode)
	}
}

func TestHandleRequestNoQuestion(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, testResolver{})

	req := new(dns.Msg)
	req.ID = 42

	rec := dnstest.NewTestRecorder()
	server.handleRequest(context.Background(), rec, req)

	require.NotNil(t, rec.Msg)
	require.NoError(t, rec.Msg.Unpack())
	assert.True(t, rec.Msg.Response)
	assert.EqualValues(t, dns.RcodeFormatError, rec.Msg.Rcode)
	assert.EqualValues(t, 42, rec.Msg.ID)
}
