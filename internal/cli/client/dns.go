package client

import (
	"context"
	"fmt"
	"uncloud/internal/machine/api/pb"
)

// TODO:
func (cli *Client) CreateIngressRecords(ctx context.Context, serviceID string) ([]*pb.DNSRecord, error) {
	// TODO:
	//  - Inspect the service and get the list of machines it runs on.
	//  - For each machine get the machine's public IP address(s).
	//  - Update the wildcard DNS record for the service with the public IP addresses (call Cluster API).

	req := &pb.CreateDomainRecordsRequest{
		Records: []*pb.DNSRecord{
			{
				Name: "*",
				Type: pb.DNSRecord_A,
				// TODO: Get the public IP addresses of the machines running Caddy containers.
				Values: []string{"1.2.3.4", "5.6.7.8"},
			},
			// TODO: Add AAAA record with routable IPv6 addresses of machines running Caddy containers.
		},
	}
	resp, err := cli.CreateDomainRecords(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create cluster domain records in Uncloud DNS: %w", err)
	}

	return resp.Records, nil
}
