package api

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/mholt/acmez/v3/acme"
	"github.com/psviderski/uncloud/api/pb"
)

// IssuedCertificate describes a certificate in Caddy's managed certificate storage.
// It does not indicate whether Caddy currently serves the certificate or whether it is trusted.
type IssuedCertificate struct {
	// SAN is the Subject Alternative Name (SAN) of the certificate.
	// Caddy doesn't issue certificates with multiple SANs.
	SAN string
	// Chain contains the parsed certificates in stored order, with the leaf first.
	Chain []*x509.Certificate
	// IssuerData is extra information associated with the certificate, usually provided by the issuer implementation.
	IssuerData CertificateIssuerData
}

// CertificateIssuerData preserves issuer-specific metadata and provides a best-effort typed view of ACME records.
type CertificateIssuerData struct {
	// Raw is the original issuer_data JSON, including unrecognized formats and fields.
	Raw json.RawMessage
	// ACME is populated when the metadata can be decoded as an ACME record with CA and certificate URLs.
	// It is nil for absent, unrecognized, or malformed metadata.
	ACME *ACMEIssuerData
}

// ACMEIssuerData identifies the ACME resources used to issue a certificate.
type ACMEIssuerData struct {
	// URL is the certificate resource URL as provisioned by the ACME server.
	URL string
	// CA is the directory URL of the ACME CA that issued this certificate.
	CA string
	// Account is the URL of the account that obtained the certificate.
	Account string
	// RenewalInfo is the stored renewal guidance, not a guarantee of when renewal will run.
	RenewalInfo *acme.RenewalInfo
}

// IssuedCertificateFromProto parses a stored certificate chain without verifying trust or expiry.
// Issuer metadata is decoded as ACME on a best-effort basis and is always preserved in its raw form.
func IssuedCertificateFromProto(p *pb.IssuedCertificate) (IssuedCertificate, error) {
	if p == nil || strings.TrimSpace(p.San) == "" {
		return IssuedCertificate{}, fmt.Errorf("invalid certificate: missing SAN")
	}
	chain, err := parseCertificateChain(p.Chain)
	if err != nil {
		return IssuedCertificate{}, fmt.Errorf("parse certificate '%s': %w", p.San, err)
	}
	return IssuedCertificate{
		SAN:        p.San,
		Chain:      chain,
		IssuerData: parseCertificateIssuerData(p.IssuerData),
	}, nil
}

func parseCertificateChain(data []byte) ([]*x509.Certificate, error) {
	var chain []*x509.Certificate
	for len(data) > 0 {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("unexpected PEM block type '%s'", block.Type)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse X.509 certificate: %w", err)
		}
		chain = append(chain, cert)
	}

	if len(chain) == 0 {
		return nil, fmt.Errorf("empty certificate chain")
	}

	return chain, nil
}

func parseCertificateIssuerData(raw json.RawMessage) CertificateIssuerData {
	data := CertificateIssuerData{Raw: raw}
	// Recognize ACME by its resource URLs and decodable metadata.
	// Unknown formats and malformed records remain raw-only.
	var cert acme.Certificate
	if err := json.Unmarshal(raw, &cert); err != nil || cert.CA == "" || cert.URL == "" {
		return data
	}
	data.ACME = &ACMEIssuerData{
		URL:         cert.URL,
		CA:          cert.CA,
		Account:     cert.Account,
		RenewalInfo: cert.RenewalInfo,
	}
	return data
}
