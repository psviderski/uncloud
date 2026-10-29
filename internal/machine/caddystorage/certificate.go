package caddystorage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/machine/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ListCertificates lists issued certificates from this machine's Caddy storage replica.
// Unreadable entries and invalid resource metadata are skipped. Chains and issuer data are returned as stored.
func (s *Server) ListCertificates(ctx context.Context, _ *emptypb.Empty) (*pb.ListCertificatesResponse, error) {
	records, err := s.store.List(ctx, "certificates/", store.KeyspaceListOptions{KeysOnly: true})
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Errorf(codes.Internal, "list Caddy certificates: %v", err)
	}

	resp := &pb.ListCertificatesResponse{}
	for _, record := range records {
		if err = ctx.Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
		if !isCertificateKey(record.Key) {
			continue
		}
		cert, err := s.loadIssuedCertificate(ctx, record.Key)
		if err != nil {
			slog.Warn("Failed to load issued certificate from Caddy storage.", "key", record.Key, "err", err)
			continue
		}
		resp.Certificates = append(resp.Certificates, cert)
	}

	return resp, nil
}

// isCertificateKey recognizes CertMagic's certificates/<issuer>/<name>/<name>.crt layout.
func isCertificateKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 4 && parts[0] == "certificates" && parts[3] == parts[2]+".crt"
}

func (s *Server) loadIssuedCertificate(ctx context.Context, key string) (*pb.IssuedCertificate, error) {
	chain, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	metaKey := strings.TrimSuffix(key, ".crt") + ".json"
	meta, err := s.store.Get(ctx, metaKey)
	if err != nil {
		return nil, err
	}

	var resource certmagic.CertificateResource
	if err = json.Unmarshal(meta.Value, &resource); err != nil {
		return nil, err
	}
	if len(resource.SANs) == 0 || strings.TrimSpace(resource.SANs[0]) == "" {
		return nil, fmt.Errorf("certificate metadata has no SAN")
	}

	return &pb.IssuedCertificate{
		San:        resource.SANs[0],
		Chain:      chain.Value,
		IssuerData: resource.IssuerData,
	}, nil
}
