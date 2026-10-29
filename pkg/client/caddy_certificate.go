package client

import (
	"context"
	"errors"

	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CaddyListCertificatesOptions controls which machine's certificate storage replica is queried.
type CaddyListCertificatesOptions struct {
	// Machine is the machine name or ID. If empty, the machine the client is connected to is used.
	Machine string
}

// ListCertificates lists issued certificates from the Caddy's managed certificate storage backed by the distributed
// cluster store. It doesn't verify trust or whether Caddy serves them and may include expired certificates.
// Private key assets are never read or returned.
//
// This is a non-atomic inventory of one store replica. It does not wait for replication.
// It returns parsing errors as a combined error, but still returns successfully read certificates regardless
// of errors.
func (c *CaddyClient) ListCertificates(ctx context.Context, opts CaddyListCertificatesOptions) ([]api.IssuedCertificate, error) {
	if opts.Machine != "" {
		ctx = ProxySingleMachineContext(ctx, opts.Machine)
	}
	resp, err := c.Storage.ListCertificates(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	var certs []api.IssuedCertificate
	var errs []error
	for _, p := range resp.Certificates {
		if cert, err := api.IssuedCertificateFromProto(p); err != nil {
			errs = append(errs, err)
		} else {
			certs = append(certs, cert)
		}
	}

	return certs, errors.Join(errs...)
}
