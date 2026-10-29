package caddystorage

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/stretchr/testify/require"
)

func TestListKeys(t *testing.T) {
	tests := []struct {
		name      string
		records   []store.Record
		prefix    string
		recursive bool
		want      []string
	}{
		{
			name:      "empty root",
			recursive: true,
		},
		{
			name: "root immediate children",
			records: []store.Record{
				{Key: "ocsp/example.com"},
				{Key: "certificates/issuer/example.com/example.com.key"},
				{Key: "certificates/issuer/example.com/example.com.crt"},
			},
			want: []string{
				"certificates",
				"ocsp",
			},
		},
		{
			name: "root recursive",
			records: []store.Record{
				{Key: "ocsp/example.com"},
				{Key: "certificates/issuer/example.com/example.com.key"},
				{Key: "certificates/issuer/example.com/example.com.crt"},
			},
			recursive: true,
			want: []string{
				"certificates",
				"certificates/issuer",
				"certificates/issuer/example.com",
				"certificates/issuer/example.com/example.com.crt",
				"certificates/issuer/example.com/example.com.key",
				"ocsp",
				"ocsp/example.com",
			},
		},
		{
			name: "nested immediate children",
			records: []store.Record{
				{Key: "certificates/issuer/example.net/example.net.crt"},
				{Key: "certificates/issuer/example.com/example.com.key"},
				{Key: "certificates/issuer/example.com/example.com.crt"},
			},
			prefix: "certificates/issuer",
			want: []string{
				"certificates/issuer/example.com",
				"certificates/issuer/example.net",
			},
		},
		{
			name: "nested terminal children",
			records: []store.Record{
				{Key: "ocsp/example.net"},
				{Key: "ocsp/example.com"},
			},
			prefix: "ocsp",
			want: []string{
				"ocsp/example.com",
				"ocsp/example.net",
			},
		},
		{
			name: "nested recursive",
			records: []store.Record{
				{Key: "certificates/issuer/example.net/example.net.crt"},
				{Key: "certificates/issuer/example.com/example.com.key"},
				{Key: "certificates/issuer/example.com/example.com.crt"},
			},
			prefix:    "certificates/issuer",
			recursive: true,
			want: []string{
				"certificates/issuer/example.com",
				"certificates/issuer/example.com/example.com.crt",
				"certificates/issuer/example.com/example.com.key",
				"certificates/issuer/example.net",
				"certificates/issuer/example.net/example.net.crt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listKeys(tt.records, tt.prefix, tt.recursive)
			if len(tt.want) == 0 {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
