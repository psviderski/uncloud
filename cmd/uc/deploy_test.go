package main

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

const webService = "web"

func testService(name, ports string) api.Service {
	return api.Service{
		Name: name,
		Containers: []api.MachineServiceContainer{
			{
				Container: api.ServiceContainer{
					Container: api.Container{
						InspectResponse: container.InspectResponse{
							Config: &container.Config{
								Labels: map[string]string{api.LabelServicePorts: ports},
							},
						},
					},
				},
			},
		},
	}
}

func TestFormatServiceEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		services []api.Service
		names    []string
		want     string
	}{
		{
			name:     "single service",
			services: []api.Service{testService(webService, "example.com:80/https")},
			names:    []string{webService},
			want:     "web endpoints:\n • https://example.com → :80\n",
		},
		{
			name:     "non-standard published port",
			services: []api.Service{testService(webService, "example.com:8443:8080/https")},
			names:    []string{webService},
			want:     "web endpoints:\n • https://example.com:8443 → :8080\n",
		},
		{
			name:     "service without http endpoints",
			services: []api.Service{testService("db", "5432/tcp")},
			names:    []string{"db"},
			want:     "",
		},
		{
			name:     "service without ports",
			services: []api.Service{testService(webService, "")},
			names:    []string{webService},
			want:     "",
		},
		{
			name: "names order is preserved",
			services: []api.Service{
				testService(webService, "example.com:80/http"),
				testService("api", "api.example.com:443/https"),
			},
			names: []string{"api", webService},
			want: "api endpoints:\n • https://api.example.com → :443\n\n" +
				"web endpoints:\n • http://example.com → :80\n",
		},
		{
			name:     "unknown service is skipped",
			services: []api.Service{testService(webService, "example.com:80/http")},
			names:    []string{"missing"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatServiceEndpoints(tt.services, tt.names))
		})
	}
}
