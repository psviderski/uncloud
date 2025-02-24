package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"uncloud/internal/dns"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/store"
)

// uncloudDNSKey is the key used to store the details of the reserved domain in the store.
const uncloudDNSKey = "uncloud_dns"

type uncloudDNSDomain struct {
	Name string
	// TODO: encrypt the token in the store.
	Token string
}

func (c *Cluster) ReserveDomain(ctx context.Context, req *pb.ReserveDomainRequest) (*pb.Domain, error) {
	if err := c.checkInitialised(ctx); err != nil {
		return nil, err
	}

	if req.ApiEndpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "API endpoint not set")
	}

	if _, err := c.storedDomain(ctx); err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "domain already reserved")
	} else {
		if s := status.Convert(err); s.Code() != codes.NotFound {
			return nil, err
		}
	}

	dnsClient := dns.NewClient()
	name, token, err := dnsClient.ReserveDomain(req.ApiEndpoint)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	domain := uncloudDNSDomain{
		Name:  name,
		Token: token,
	}
	domainJSON, err := json.Marshal(domain)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal reserved domain for store: %v", err)
	}
	if err = c.store.Put(ctx, uncloudDNSKey, domainJSON); err != nil {
		return nil, status.Errorf(codes.Internal, "store reserved domain: %v", err)
	}

	return &pb.Domain{Name: name}, nil
}

func (c *Cluster) GetDomain(ctx context.Context, _ *emptypb.Empty) (*pb.Domain, error) {
	if err := c.checkInitialised(ctx); err != nil {
		return nil, err
	}

	domain, err := c.storedDomain(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.Domain{Name: domain.Name}, nil
}

func (c *Cluster) storedDomain(ctx context.Context) (uncloudDNSDomain, error) {
	var domain uncloudDNSDomain
	var domainJSON []byte

	if err := c.store.Get(ctx, uncloudDNSKey, &domainJSON); err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			return domain, status.Errorf(codes.NotFound, "domain not found")
		}
		return domain, status.Errorf(codes.Internal, "get domain from store: %v", err)
	}

	if err := json.Unmarshal(domainJSON, &domain); err != nil {
		return domain, status.Errorf(codes.Internal, "unmarshal domain: %v", err)
	}

	return domain, nil
}

func (c *Cluster) ReleaseDomain(ctx context.Context, _ *emptypb.Empty) (*pb.Domain, error) {
	if err := c.checkInitialised(ctx); err != nil {
		return nil, err
	}

	domain, err := c.storedDomain(ctx)
	if err != nil {
		return nil, err
	}

	if err = c.store.Delete(ctx, uncloudDNSKey); err != nil {
		return nil, status.Errorf(codes.Internal, "delete domain from store: %v", err)
	}
	// TODO: implement and call Uncloud DNS endpoint to release/delete the domain.

	return &pb.Domain{Name: domain.Name}, nil
}
