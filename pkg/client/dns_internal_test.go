package client

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetCreateDomainRecordsRequest(t *testing.T) {
	tests := map[string]struct {
		given   []*pb.MachineInfo
		wantReq *pb.CreateDomainRecordsRequest
		wantErr error
	}{
		"single IPv4 machine": {
			given: []*pb.MachineInfo{
				{Id: "m1", Name: "machine-1", PublicIp: pb.NewIP(netip.MustParseAddr("1.2.3.4"))},
			},
			wantReq: &pb.CreateDomainRecordsRequest{
				Records: []*pb.DNSRecord{
					{Name: "*", Type: pb.DNSRecord_A, Values: []string{"1.2.3.4"}},
				},
			},
		},
		"single IPv6 machine": {
			given: []*pb.MachineInfo{
				{Id: "m1", Name: "machine-1", PublicIp: pb.NewIP(netip.MustParseAddr("2001:db8::1"))},
			},
			wantReq: &pb.CreateDomainRecordsRequest{
				Records: []*pb.DNSRecord{
					{Name: "*", Type: pb.DNSRecord_AAAA, Values: []string{"2001:db8::1"}},
				},
			},
		},
		"multiple IPv4 machines": {
			given: []*pb.MachineInfo{
				{Id: "m1", Name: "machine-1", PublicIp: pb.NewIP(netip.MustParseAddr("1.2.3.4"))},
				{Id: "m2", Name: "machine-2", PublicIp: pb.NewIP(netip.MustParseAddr("5.6.7.8"))},
			},
			wantReq: &pb.CreateDomainRecordsRequest{
				Records: []*pb.DNSRecord{
					{Name: "*", Type: pb.DNSRecord_A, Values: []string{"1.2.3.4", "5.6.7.8"}},
				},
			},
		},
		"mixed IPv4 and IPv6 machines": {
			given: []*pb.MachineInfo{
				{Id: "m1", Name: "machine-1", PublicIp: pb.NewIP(netip.MustParseAddr("1.2.3.4"))},
				{Id: "m2", Name: "machine-2", PublicIp: pb.NewIP(netip.MustParseAddr("2001:db8::1"))},
				{Id: "m3", Name: "machine-3", PublicIp: pb.NewIP(netip.MustParseAddr("10.0.0.1"))},
				{Id: "m4", Name: "machine-4", PublicIp: pb.NewIP(netip.MustParseAddr("2001:db8::2"))},
			},
			wantReq: &pb.CreateDomainRecordsRequest{
				Records: []*pb.DNSRecord{
					{Name: "*", Type: pb.DNSRecord_A, Values: []string{"1.2.3.4", "10.0.0.1"}},
					{Name: "*", Type: pb.DNSRecord_AAAA, Values: []string{"2001:db8::1", "2001:db8::2"}},
				},
			},
		},
		"empty machine list": {
			given:   nil,
			wantErr: fmt.Errorf("at least one machine must be provided"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			gotReq, gotErr := getCreateDomainRecordsRequest(tt.given)
			if tt.wantErr == nil {
				require.NoError(t, gotErr)
				assert.Equal(t, tt.wantReq, gotReq)
			} else {
				assert.Nil(t, gotReq)
				assert.Equal(t, tt.wantErr.Error(), gotErr.Error())
			}
		})
	}
}
