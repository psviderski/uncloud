package cert

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintCertificates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	certs := []api.IssuedCertificate{
		testIssuedCertificate("z.example.com", "z", now.Add(48*time.Hour), certmagic.LetsEncryptProductionCA),
		testIssuedCertificate("a.example.com", "old", now.Add(-48*time.Hour), certmagic.LetsEncryptStagingCA),
		testIssuedCertificate("a.example.com", "new", now.Add(72*time.Hour), "https://ca.example/directory"),
		testIssuedCertificate("unknown.example.com", "unknown", now.Add(96*time.Hour), ""),
	}

	var table bytes.Buffer
	require.NoError(t, printCertificates(&table, certs, ""))
	output := table.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ISSUER")
	assert.Contains(t, output, "EXPIRES")
	assert.Contains(t, output, now.Add(-48*time.Hour).Format(time.DateOnly)+" (expired ")
	assert.Contains(t, output, now.Add(48*time.Hour).Format(time.DateOnly)+" (")
	assert.Contains(t, output, "Let's Encrypt (staging)")
	assert.Contains(t, output, "https://ca.example/directory")
	assert.Less(t, strings.Index(output, now.Add(-48*time.Hour).Format(time.DateOnly)),
		strings.Index(output, now.Add(72*time.Hour).Format(time.DateOnly)))
	assert.Less(t, strings.Index(output, "a.example.com"), strings.Index(output, "z.example.com"))
	assert.Contains(t, output, testFingerprint("old")[:12])

	var encoded bytes.Buffer
	require.NoError(t, printCertificates(&encoded, certs, "json"))
	var items []struct {
		api.IssuedCertificate
		Fingerprint string
	}
	require.NoError(t, json.Unmarshal(encoded.Bytes(), &items))
	require.Len(t, items, 4)
	assert.Equal(t, testFingerprint("old"), items[0].Fingerprint)
	assert.Equal(t, "a.example.com", items[0].SAN)
	require.Len(t, items[0].Chain, 1)
	assert.Equal(t, []byte("old"), items[0].Chain[0].Raw)
	assert.Equal(t, now.Add(-48*time.Hour), items[0].Chain[0].NotAfter)
	require.NotNil(t, items[0].IssuerData.ACME)
	assert.Equal(t, certmagic.LetsEncryptStagingCA, items[0].IssuerData.ACME.CA)
	assert.Equal(t, testFingerprint("new"), items[1].Fingerprint)
	require.NotNil(t, items[1].IssuerData.ACME)
	assert.Equal(t, "https://ca.example/directory", items[1].IssuerData.ACME.CA)
	assert.Nil(t, items[2].IssuerData.ACME)
	assert.Equal(t, "z.example.com", items[3].SAN)
}

func TestPrintCertificates_Empty(t *testing.T) {
	for _, tt := range []struct {
		output string
		want   string
	}{
		{output: "", want: "No Caddy certificates found in cluster storage.\n"},
		{output: "json", want: "[]\n"},
	} {
		var out bytes.Buffer
		require.NoError(t, printCertificates(&out, nil, tt.output))
		assert.Equal(t, tt.want, out.String())
	}
}

func TestFormatIssuer(t *testing.T) {
	for _, tt := range []struct {
		name string
		ca   string
		want string
	}{
		{name: "production", ca: certmagic.LetsEncryptProductionCA, want: "Let's Encrypt"},
		{name: "staging", ca: certmagic.LetsEncryptStagingCA, want: "Let's Encrypt (staging)"},
		{name: "ZeroSSL", ca: certmagic.ZeroSSLProductionCA, want: "ZeroSSL"},
		{name: "Google Trust Services", ca: certmagic.GoogleTrustProductionCA, want: "Google Trust Services"},
		{name: "Google Trust Services staging", ca: certmagic.GoogleTrustStagingCA, want: "Google Trust Services (staging)"},
		{name: "other", ca: "https://ca.example/directory", want: "https://ca.example/directory"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatIssuer(api.CertificateIssuerData{
				ACME: &api.ACMEIssuerData{CA: tt.ca},
			}))
		})
	}

	assert.Equal(t, "unknown", formatIssuer(api.CertificateIssuerData{}))
}

func TestFormatExpiry_UTC(t *testing.T) {
	now := time.Now().UTC()
	brisbane := time.FixedZone("AEST", 10*60*60)
	expires := time.Date(now.Year(), now.Month(), now.Day()+3, 3, 0, 0, 0, brisbane)
	assert.True(t, strings.HasPrefix(formatExpiry(expires), expires.UTC().Format(time.DateOnly)+" ("))
	assert.NotEqual(t, expires.Format(time.DateOnly), expires.UTC().Format(time.DateOnly))
	expired := now.Add(-2 * time.Hour)
	assert.True(t, strings.HasPrefix(formatExpiry(expired), expired.UTC().Format(time.DateOnly)+" (expired "))
	assert.Contains(t, formatExpiry(expired), " ago)")
}

func testIssuedCertificate(name, raw string, expires time.Time, ca string) api.IssuedCertificate {
	cert := api.IssuedCertificate{
		SAN:   name,
		Chain: []*x509.Certificate{{Raw: []byte(raw), NotAfter: expires}},
	}
	if ca != "" {
		cert.IssuerData.ACME = &api.ACMEIssuerData{CA: ca}
	}
	return cert
}

func testFingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
