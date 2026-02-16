package client

import (
	"net/netip"
	"testing"
)

func TestGetVerifyURL(t *testing.T) {
	tests := map[string]struct {
		ip   netip.Addr
		want string
	}{
		"IPv4": {
			ip:   netip.MustParseAddr("93.184.216.34"),
			want: "http://93.184.216.34:/.uncloud-verify",
		},
		"IPv6": {
			ip:   netip.MustParseAddr("2001:db8::1"),
			want: "http://[2001:db8::1]:/.uncloud-verify",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := getVerifyURL(tt.ip); got != tt.want {
				t.Errorf("getVerifyURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
