package client

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVerifyURL(t *testing.T) {
	tests := map[string]struct {
		given netip.Addr
		want  string
	}{
		"IPv4": {
			given: netip.MustParseAddr("93.184.216.34"),
			want:  "http://93.184.216.34:/.uncloud-verify",
		},
		"IPv6": {
			given: netip.MustParseAddr("2001:db8::1"),
			want:  "http://[2001:db8::1]:/.uncloud-verify",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := getVerifyURL(tt.given)
			assert.Equal(t, tt.want, got)
		})
	}
}
