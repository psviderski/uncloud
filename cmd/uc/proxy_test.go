package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const httpScheme = "http://"

func TestSchemeForPort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		forceHTTP bool
		want      string
	}{
		{
			name: "known http port",
			port: 8080,
			want: httpScheme,
		},
		{
			name: "unknown http port",
			port: 9001,
			want: "",
		},
		{
			name: "https port",
			port: 443,
			want: "",
		},
		{
			name:      "unknown http port forced to http",
			port:      9001,
			forceHTTP: true,
			want:      httpScheme,
		},
		{
			name:      "https port forced to http",
			port:      443,
			forceHTTP: true,
			want:      httpScheme,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, schemeForPort(tt.port, tt.forceHTTP))
		})
	}
}
