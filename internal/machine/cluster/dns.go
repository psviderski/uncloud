package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/miekg/dns"
	"github.com/psviderski/uncloud/api/pb"
	undns "github.com/psviderski/uncloud/internal/dns"
	"github.com/psviderski/uncloud/internal/machine/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// uncloudDNSKey stores the cluster domain and, when reserved, its Uncloud DNS credentials.
const uncloudDNSKey = "uncloud_dns"

type uncloudDNSDomain struct {
	// Endpoint is the API endpoint of the Uncloud DNS service where the domain is reserved.
	// An empty endpoint means the domain is managed externally.
	Endpoint string
	Name     string
	// TODO: encrypt the token in the store.
	Token string
}

func (c *Cluster) ReserveDomain(ctx context.Context, req *pb.ReserveDomainRequest) (*pb.Domain, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	if req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "API endpoint not set")
	}

	if _, err := c.storedDomain(ctx); err == nil {
		return nil, status.Error(codes.AlreadyExists, "cluster domain already configured")
	} else {
		if s := status.Convert(err); s.Code() != codes.NotFound {
			return nil, err
		}
	}

	dnsClient := undns.NewClient()
	name, token, err := dnsClient.ReserveDomain(req.Endpoint)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	domain := uncloudDNSDomain{
		Endpoint: req.Endpoint,
		Name:     name,
		Token:    token,
	}
	domainJSON, err := json.Marshal(domain)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal reserved domain for store: %v", err)
	}
	if err = c.store.Put(ctx, uncloudDNSKey, domainJSON); err != nil {
		return nil, status.Errorf(codes.Internal, "store reserved domain: %v", err)
	}

	return &pb.Domain{Name: name, Reserved: proto.Bool(true)}, nil
}

func (c *Cluster) GetDomain(ctx context.Context, _ *emptypb.Empty) (*pb.Domain, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	domain, err := c.storedDomain(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.Domain{Name: domain.Name, Reserved: proto.Bool(domain.Endpoint != "")}, nil
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
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	domain, err := c.storedDomain(ctx)
	if err != nil {
		return nil, err
	}
	if domain.Endpoint == "" {
		return nil, status.Error(codes.FailedPrecondition,
			"cluster domain is set manually, use 'uc dns set \"\"' to unset it")
	}

	if err = c.store.Delete(ctx, uncloudDNSKey); err != nil {
		return nil, status.Errorf(codes.Internal, "delete domain from store: %v", err)
	}
	// TODO: implement and call Uncloud DNS endpoint to release/delete the domain.

	return &pb.Domain{Name: domain.Name, Reserved: proto.Bool(true)}, nil
}

func (c *Cluster) SetDomain(ctx context.Context, req *pb.SetDomainRequest) (*emptypb.Empty, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	name := req.GetName()
	if name != "" {
		if labels, ok := dns.IsDomainName(name); !ok || labels < 2 {
			return nil, status.Errorf(codes.InvalidArgument,
				"invalid cluster domain '%s': must be a valid domain name with at least two labels", name)
		}
	}
	name = strings.ToLower(strings.TrimSuffix(name, "."))

	existing, err := c.storedDomain(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if name == "" {
		if err == nil {
			if existing.Endpoint != "" {
				return nil, status.Error(codes.FailedPrecondition,
					"cluster domain is reserved in Uncloud DNS, use 'uc dns release' to release it")
			}
			if err := c.store.Delete(ctx, uncloudDNSKey); err != nil {
				return nil, status.Errorf(codes.Internal, "unset cluster domain: %v", err)
			}
		}
		return &emptypb.Empty{}, nil
	}
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "cluster domain already configured")
	}

	domain := uncloudDNSDomain{Name: name}
	domainJSON, err := json.Marshal(domain)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal set domain for store: %v", err)
	}
	if err = c.store.Put(ctx, uncloudDNSKey, domainJSON); err != nil {
		return nil, status.Errorf(codes.Internal, "store set domain: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (c *Cluster) CreateDomainRecords(
	ctx context.Context, req *pb.CreateDomainRecordsRequest,
) (*pb.CreateDomainRecordsResponse, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	domain, err := c.storedDomain(ctx)
	if err != nil {
		return nil, err
	}

	if domain.Endpoint == "" {
		return nil, status.Error(codes.FailedPrecondition, "cluster domain is not reserved in Uncloud DNS")
	}

	dnsClient := undns.NewClient()
	recordsReq := make([]undns.RecordRequest, len(req.Records))
	for i, r := range req.Records {
		recordsReq[i] = undns.RecordRequest{
			Name:   r.Name,
			Type:   undns.RecordType(r.Type.String()),
			Values: r.Values,
		}
	}

	recordsResp, err := dnsClient.CreateRecords(domain.Endpoint, domain.Name, domain.Token, recordsReq)
	if err != nil {
		return nil, err
	}

	resp := &pb.CreateDomainRecordsResponse{
		Records: make([]*pb.DNSRecord, len(recordsResp)),
	}
	for i, r := range recordsResp {
		resp.Records[i] = &pb.DNSRecord{
			Name:   r.FQDN,
			Values: r.Values,
		}

		switch r.Type {
		case undns.RecordTypeA:
			resp.Records[i].Type = pb.DNSRecord_A
		case undns.RecordTypeAAAA:
			resp.Records[i].Type = pb.DNSRecord_AAAA
		}
	}

	return resp, nil
}
