package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/mholt/acmez/v3/acme"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssuedCertificateFromProto(t *testing.T) {
	leaf := testCertificatePEM(t, 1)
	issuer := testCertificatePEM(t, 2)
	for _, tt := range []struct {
		name    string
		chain   []byte
		serials []int64
	}{
		{name: "single certificate", chain: leaf, serials: []int64{1}},
		{name: "certificate chain", chain: append(append([]byte(nil), leaf...), issuer...), serials: []int64{1, 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := IssuedCertificateFromProto(&pb.IssuedCertificate{
				San: "app.example.com", Chain: tt.chain,
			})
			require.NoError(t, err)
			assert.Equal(t, "app.example.com", cert.SAN)
			require.Len(t, cert.Chain, len(tt.serials))

			for i, serial := range tt.serials {
				assert.Equal(t, big.NewInt(serial), cert.Chain[i].SerialNumber, "chain order must be preserved")
			}
			assert.Equal(t, []string{"app.example.com"}, cert.Chain[0].DNSNames)
			// Expired, self-signed certificates are parsed without verifying trust or validity.
			assert.Equal(t, time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), cert.Chain[0].NotAfter)
			assert.Empty(t, cert.IssuerData)
		})
	}
}

func TestIssuedCertificateFingerprint(t *testing.T) {
	cert := IssuedCertificate{Chain: []*x509.Certificate{
		{Raw: []byte("abc")},
		{Raw: []byte("issuer")},
	}}
	want, err := hex.DecodeString("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	require.NoError(t, err)
	fingerprint := cert.Fingerprint()
	assert.Equal(t, want, fingerprint[:])
}

func TestIssuedCertificateFromProto_InvalidCertificate(t *testing.T) {
	valid := testCertificatePEM(t, 1)
	invalidDER := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")})
	for _, tt := range []struct {
		name    string
		input   *pb.IssuedCertificate
		wantErr string
	}{
		{name: "nil input", wantErr: "missing SAN"},
		{name: "missing SAN", input: &pb.IssuedCertificate{Chain: valid}, wantErr: "missing SAN"},
		{name: "blank SAN", input: &pb.IssuedCertificate{San: " \t", Chain: valid}, wantErr: "missing SAN"},
		{name: "empty chain", input: &pb.IssuedCertificate{San: "app.example.com"}, wantErr: "empty certificate chain"},
		{
			name: "invalid PEM", input: &pb.IssuedCertificate{San: "app.example.com", Chain: []byte("not PEM")},
			wantErr: "empty certificate chain",
		},
		{
			name: "wrong PEM block type", input: &pb.IssuedCertificate{
				San:   "app.example.com",
				Chain: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("key")}),
			},
			wantErr: "unexpected PEM block type",
		},
		{
			name: "invalid DER", input: &pb.IssuedCertificate{San: "app.example.com", Chain: invalidDER},
			wantErr: "parse X.509 certificate",
		},
		{name: "invalid certificate after valid leaf", input: &pb.IssuedCertificate{
			San:   "app.example.com",
			Chain: append(append([]byte(nil), valid...), invalidDER...),
		}, wantErr: "parse X.509 certificate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := IssuedCertificateFromProto(tt.input)
			require.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, cert)
		})
	}
}

func TestIssuedCertificateFromProto_IssuerData(t *testing.T) {
	chain := testCertificatePEM(t, 1)
	retryAfter := time.Date(2029, 12, 1, 0, 0, 0, 0, time.UTC)
	renewalInfo := &acme.RenewalInfo{
		ExplanationURL:   "https://ca.example/why",
		UniqueIdentifier: "aki.serial",
		RetryAfter:       &retryAfter,
		SelectedTime:     time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	renewalInfo.SuggestedWindow.Start = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	renewalInfo.SuggestedWindow.End = time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name string
		raw  string
		want *ACMEIssuerData
	}{
		{
			name: "ACME",
			raw:  `{"url":"https://ca.example/cert/1","ca":"https://ca.example/directory","account":"https://ca.example/acct/1","future_field":true}`,
			want: &ACMEIssuerData{
				URL: "https://ca.example/cert/1", CA: "https://ca.example/directory", Account: "https://ca.example/acct/1",
			},
		},
		{
			name: "ACME with renewal information",
			raw: `{
				"url": "https://ca.example/cert/1",
				"ca": "https://ca.example/directory",
				"account": "https://ca.example/acct/1",
				"renewal_info": {
					"suggestedWindow": {"start": "2030-01-01T00:00:00Z", "end": "2030-01-03T00:00:00Z"},
					"explanationURL": "https://ca.example/why",
					"_uniqueIdentifier": "aki.serial",
					"_retryAfter": "2029-12-01T00:00:00Z",
					"_selectedTime": "2030-01-02T00:00:00Z"
				}
			}`,
			want: &ACMEIssuerData{
				URL: "https://ca.example/cert/1", CA: "https://ca.example/directory",
				Account: "https://ca.example/acct/1", RenewalInfo: renewalInfo,
			},
		},
		{name: "absent"},
		{name: "null", raw: "null"},
		{name: "unknown issuer", raw: `{"id":"provider-id","status":"issued"}`},
		{name: "non-object JSON", raw: `"provider-record"`},
		{name: "missing CA", raw: `{"url":"https://ca.example/cert/1"}`},
		{name: "missing certificate URL", raw: `{"ca":"https://ca.example/directory"}`},
		{name: "invalid JSON", raw: "{"},
		{name: "invalid field types", raw: `{"url":123,"ca":false}`},
		{
			name: "malformed renewal information",
			raw:  `{"url":"https://ca.example/cert/1","ca":"https://ca.example/directory","renewal_info":{"_selectedTime":"invalid"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := IssuedCertificateFromProto(&pb.IssuedCertificate{
				San: "app.example.com", Chain: chain, IssuerData: []byte(tt.raw),
			})
			require.NoError(t, err, "issuer metadata must not prevent certificate parsing")
			require.Len(t, cert.Chain, 1)
			assert.Equal(t, tt.raw, string(cert.IssuerData.Raw), "preserve original JSON including unknown fields")
			assert.Equal(t, tt.want, cert.IssuerData.ACME)
		})
	}
}

// testCertificatePEM creates an expired, self-signed certificate for parsing tests.
func testCertificatePEM(t *testing.T, serial int64) []byte {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		DNSNames:     []string{"app.example.com"},
		NotBefore:    time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
