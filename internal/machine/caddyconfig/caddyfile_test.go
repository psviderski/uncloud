package caddyconfig

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaddyfileGenerator(t *testing.T) {
	caddyfileHeader := `http:// {
	handle /.uncloud-verify {
		respond "test-machine-id" 200
	}
	log
}

(common_proxy) {
	# Retry failed requests up to lb_retries times against other available upstreams.
	lb_retries 3
	# Upstreams are marked unhealthy for fail_duration after a failed request (passive health checking).
	fail_duration 30s
}
`

	// TODO: mock validator
	generator := NewCaddyfileGenerator("test-machine-id", nil, nil)

	tests := []struct {
		name       string
		containers []store.ContainerRecord
		want       string
		wantErr    bool
	}{
		{
			name:       "empty containers",
			containers: []store.ContainerRecord{},
			want:       caddyfileHeader,
		},
		{
			name: "HTTP container",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainer("10.210.0.2", "app.example.com:8080/http"), "mach1"),
			},
			want: caddyfileHeader + `
http://app.example.com {
	reverse_proxy {
		to 10.210.0.2:8080
		import common_proxy
	}
	log
}
`,
		},
		{
			name: "load balancing multiple containers",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainer("10.210.0.2", "app.example.com:8080/http"), "mach1"),
				newContainerRecord(newContainer("10.210.0.3", "app.example.com:8080/http"), "mach1"),
			},
			want: caddyfileHeader + `
http://app.example.com {
	reverse_proxy {
		to 10.210.0.2:8080 10.210.0.3:8080
		import common_proxy
	}
	log
}
`,
		},
		{
			name: "HTTPS container",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainer("10.210.0.2", "secure.example.com:8000/https"), "mach1"),
			},
			want: caddyfileHeader + `
https://secure.example.com {
	reverse_proxy {
		to 10.210.0.2:8000
		import common_proxy
	}
	log
}
`,
		},
		{
			name: "mixed HTTP and HTTPS",
			containers: []store.ContainerRecord{
				newContainerRecord(
					newContainer("10.210.0.2",
						"app.example.com:8080/http",
						"web.example.com:8000/http"),
					"mach1",
				),
				newContainerRecord(
					newContainer("10.210.0.3",
						"app.example.com:8080/http",
						"secure.example.com:8888/https"),
					"mach1",
				),
				newContainerRecord(
					newContainer("10.210.0.4",
						"web.example.com:8000/http",
						"secure.example.com:8888/https"),
					"mach1",
				),
				newContainerRecord(
					newContainer("10.210.0.5",
						"app.example.com:8080/http",
						"web.example.com:8000/http",
						"secure.example.com:8888/https"),
					"mach1",
				),
			},
			want: caddyfileHeader + `
http://app.example.com {
	reverse_proxy {
		to 10.210.0.2:8080 10.210.0.3:8080 10.210.0.5:8080
		import common_proxy
	}
	log
}

http://web.example.com {
	reverse_proxy {
		to 10.210.0.2:8000 10.210.0.4:8000 10.210.0.5:8000
		import common_proxy
	}
	log
}

https://secure.example.com {
	reverse_proxy {
		to 10.210.0.3:8888 10.210.0.4:8888 10.210.0.5:8888
		import common_proxy
	}
	log
}
`,
		},
		{
			name: "container without uncloud network ignored",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainerWithoutNetwork("ignored.example.com:8080/http"), "mach1"),
			},
			want: caddyfileHeader,
		},
		{
			name: "container with invalid port ignored",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainer("10.210.0.2", "invalid-port"), "mach1"),
			},
			want: caddyfileHeader,
		},
		{
			name: "containers with unsupported protocols and host mode ignored",
			containers: []store.ContainerRecord{
				newContainerRecord(newContainer("10.210.0.2", "5000/tcp"), "mach1"),
				newContainerRecord(newContainer("10.210.0.3", "5000/udp"), "mach1"),
				newContainerRecord(newContainer("10.210.0.4", "80:8080/tcp@host"), "mach1"),
			},
			want: caddyfileHeader,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := generator.Generate(ctx, tt.containers)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.want, config, "Generated Caddyfile doesn't match")
		})
	}
}

func newContainerRecord(ctr api.ServiceContainer, machineID string) store.ContainerRecord {
	return store.ContainerRecord{
		Container: ctr,
		MachineID: machineID,
	}
}
