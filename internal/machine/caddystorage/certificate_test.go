package caddystorage

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServerListCertificates(t *testing.T) {
	for _, sans := range [][]string{
		{"app.example.com"}, {"*.example.com"}, {"192.0.2.1"}, {"www.example.com", "example.com"},
	} {
		t.Run(sans[0], func(t *testing.T) {
			bundle, _, _ := certificateFixture(t, &x509.Certificate{
				DNSNames:    []string{"example.com", "www.example.com"},
				IPAddresses: []net.IP{net.ParseIP("192.0.2.1")},
				NotAfter:    time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			})
			key := certmagic.StorageKeys.SiteCert("local", sans[0])
			storage := newCertificateStorage(t)
			raw := json.RawMessage(`{"unknown_issuer_field":true}`)
			storage.add(t, key, bundle, sans, raw)
			storage.keys = append(storage.keys, "certificates", "certificates/local", "certificates/local/app.crt",
				"certificates/local/app/wrong.crt", "certificates/local/app/app.crt/extra.crt",
				"pki/authorities/local/root.crt", "ocsp/certificate")

			resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
			require.NoError(t, err)
			require.Len(t, resp.Certificates, 1)
			cert := resp.Certificates[0]
			assert.Equal(t, sans[0], cert.San)
			assert.Equal(t, bundle, cert.Chain, "PEM must be returned unchanged")
			assert.Equal(t, []byte(raw), cert.IssuerData)
			assert.Equal(t, []string{key, strings.TrimSuffix(key, ".crt") + ".json"}, storage.loads)
		})
	}
}

func TestServerListCertificates_PreservesIssuerData(t *testing.T) {
	bundle, _, _ := certificateFixture(t, &x509.Certificate{})
	for _, raw := range []string{
		"", "null", `{"url":"https://ca/cert/1","ca":"https://ca/directory"}`,
		`{"url":"https://ca/cert/1","ca":"https://ca/directory","renewal_info":{"_selectedTime":"invalid"}}`,
		`"provider-record"`, "[1,2,3]",
	} {
		t.Run(raw, func(t *testing.T) {
			storage := newCertificateStorage(t)
			storage.add(t, "certificates/custom/app/app.crt", bundle, []string{"app"}, json.RawMessage(raw))

			resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
			require.NoError(t, err)
			require.Len(t, resp.Certificates, 1)
			assert.Equal(t, raw, string(resp.Certificates[0].IssuerData))
		})
	}
}

func TestServerListCertificates_OrderingAndDuplicates(t *testing.T) {
	bundle, _, _ := certificateFixture(t, &x509.Certificate{})
	storage := newCertificateStorage(t)
	storage.add(t, certmagic.StorageKeys.SiteCert("z-issuer", "a.example.com"), bundle, []string{"a.example.com"}, nil)
	storage.add(t, certmagic.StorageKeys.SiteCert("a-issuer", "z.example.com"), bundle, []string{"z.example.com"}, nil)
	storage.add(t, certmagic.StorageKeys.SiteCert("b-issuer", "z.example.com"), bundle, []string{"z.example.com"}, nil)

	resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Len(t, resp.Certificates, 3)
	certs := resp.Certificates
	assert.Equal(t, []string{"z.example.com", "z.example.com", "a.example.com"},
		[]string{certs[0].San, certs[1].San, certs[2].San})
	for _, cert := range certs {
		assert.Equal(t, bundle, cert.Chain)
	}
}

func TestServerListCertificates_SkipsUnreadableEntries(t *testing.T) {
	bundle, _, _ := certificateFixture(t, &x509.Certificate{})
	const badKey = "certificates/local/bad/bad.crt"
	const metaKey = "certificates/local/bad/bad.json"
	for _, tt := range []struct {
		name   string
		change func(*certificateStorageStub)
	}{
		{"missing chain", func(s *certificateStorageStub) { delete(s.values, badKey) }},
		{"chain read failure", func(s *certificateStorageStub) { s.loadErrors[badKey] = errors.New("offline") }},
		{"missing metadata", func(s *certificateStorageStub) { delete(s.values, metaKey) }},
		{"metadata read failure", func(s *certificateStorageStub) { s.loadErrors[metaKey] = errors.New("offline") }},
		{"invalid JSON", func(s *certificateStorageStub) { s.values[metaKey] = []byte("{") }},
		{"missing SAN", func(s *certificateStorageStub) { s.values[metaKey] = []byte(`{}`) }},
		{"empty first SAN", func(s *certificateStorageStub) { s.values[metaKey] = []byte(`{"sans":["","app"]}`) }},
		{"blank first SAN", func(s *certificateStorageStub) { s.values[metaKey] = []byte(`{"sans":[" "]}`) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := newCertificateStorage(t)
			storage.add(t, badKey, bundle, []string{"bad"}, nil)
			storage.add(t, "certificates/local/good/good.crt", bundle, []string{"good"}, nil)
			tt.change(storage)

			resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
			require.NoError(t, err)
			require.Len(t, resp.Certificates, 1)
			assert.Equal(t, "good", resp.Certificates[0].San)
		})
	}

	t.Run("all entries unreadable", func(t *testing.T) {
		storage := newCertificateStorage(t)
		storage.keys = []string{badKey}
		resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
		require.NoError(t, err)
		assert.Empty(t, resp.Certificates)
	})
}

func TestServerListCertificates_ListErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		storage := newCertificateStorage(t)
		if fail {
			storage.listError = errors.New("offline")
		}

		resp, err := NewServer(storage).ListCertificates(t.Context(), &emptypb.Empty{})
		assert.Empty(t, storage.loads)
		if fail {
			assert.Nil(t, resp)
			assert.Equal(t, codes.Internal, status.Code(err))
		} else {
			require.NoError(t, err)
			assert.Empty(t, resp.Certificates)
		}
	}
}

// Only reads are implemented. Unexpected writes panic through the nil embedded Keyspace.
type certificateStorageStub struct {
	*store.Keyspace
	t          *testing.T
	keys       []string
	values     map[string][]byte
	loads      []string
	listCalls  int
	listError  error
	loadErrors map[string]error
	onLoad     func(string)
}

func newCertificateStorage(t *testing.T) *certificateStorageStub {
	return &certificateStorageStub{t: t, values: make(map[string][]byte), loadErrors: make(map[string]error)}
}

func (s *certificateStorageStub) add(t *testing.T, key string, bundle []byte, sans []string, issuerData json.RawMessage) {
	t.Helper()
	metaKey := strings.TrimSuffix(key, ".crt") + ".json"
	keyKey := strings.TrimSuffix(key, ".crt") + ".key"
	meta, err := json.Marshal(certmagic.CertificateResource{SANs: sans, IssuerData: issuerData})
	require.NoError(t, err)
	s.keys = append(s.keys, key, metaKey, keyKey)
	s.values[key], s.values[metaKey], s.values[keyKey] = bundle, meta, []byte("must not read private keys")
}

func (s *certificateStorageStub) List(ctx context.Context, prefix string, opts store.KeyspaceListOptions) ([]store.Record, error) {
	s.listCalls++
	assert.Equal(s.t, "certificates/", prefix)
	assert.True(s.t, opts.KeysOnly, "listing must not fetch private key values")
	var records []store.Record
	for _, key := range slices.Sorted(slices.Values(s.keys)) {
		records = append(records, store.Record{Key: key})
	}
	return records, s.listError
}

func (s *certificateStorageStub) Get(ctx context.Context, key string) (store.Record, error) {
	require.False(s.t, strings.HasSuffix(key, ".key"), "private keys must never be loaded")
	s.loads = append(s.loads, key)
	if s.onLoad != nil {
		s.onLoad(key)
	}
	if err := s.loadErrors[key]; err != nil {
		return store.Record{}, err
	}
	value, ok := s.values[key]
	if !ok {
		return store.Record{}, store.ErrKeyNotFound
	}
	return store.Record{Key: key, Value: value}, nil
}

func certificateFixture(t *testing.T, template *x509.Certificate) ([]byte, *x509.Certificate, *x509.Certificate) {
	t.Helper()
	publicKey, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	root := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		NotBefore: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, publicKey, key)
	require.NoError(t, err)
	root, err = x509.ParseCertificate(rootDER)
	require.NoError(t, err)
	template.SerialNumber = big.NewInt(2)
	template.NotBefore = root.NotBefore
	if template.NotAfter.IsZero() {
		template.NotAfter = root.NotAfter
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, template, root, publicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	bundle := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...)
	return bundle, leaf, root
}
