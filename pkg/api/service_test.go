package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceSpec_Validate_CaddyAndPorts(t *testing.T) {
	tests := []struct {
		name    string
		spec    ServiceSpec
		wantErr string
	}{
		{
			name: "valid with neither Caddy nor Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
			},
			wantErr: "",
		},
		{
			name: "valid with Caddy only",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
			},
			wantErr: "",
		},
		{
			name: "valid with Ports only",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid with empty Caddy config and Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
					},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid with both Caddy and Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
					},
				},
			},
			wantErr: "ports and Caddy configuration cannot be specified simultaneously",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
