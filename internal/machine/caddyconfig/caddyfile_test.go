package caddyconfig

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	reverse_proxy 10.210.0.2:8080 {
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
	reverse_proxy 10.210.0.2:8080 10.210.0.3:8080 {
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
	reverse_proxy 10.210.0.2:8000 {
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
	reverse_proxy 10.210.0.2:8080 10.210.0.3:8080 10.210.0.5:8080 {
		import common_proxy
	}
	log
}

http://web.example.com {
	reverse_proxy 10.210.0.2:8000 10.210.0.4:8000 10.210.0.5:8000 {
		import common_proxy
	}
	log
}

https://secure.example.com {
	reverse_proxy 10.210.0.3:8888 10.210.0.4:8888 10.210.0.5:8888 {
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
			// Validator is not expected to be called in these tests.
			generator := NewCaddyfileGenerator("test-machine-id", nil, nil)

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

func TestCaddyfileGeneratorWithCustomConfigs(t *testing.T) {
	caddyfileBase := `http:// {
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

	tests := []struct {
		name       string
		containers []store.ContainerRecord
		want       string
		wantErr    bool
	}{
		{
			name: "caddy service with valid global config",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.0.2",
					`# Global Caddy configuration
{
	global directive
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: `# Global Caddy configuration
{
	global directive
}

` + caddyfileBase,
		},
		{
			name: "regular service with valid custom config",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.0.2",
					`# Custom config for web service
web.example.com {
	reverse_proxy web:3000
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase + `
# Service: web
# Custom config for web service
web.example.com {
	reverse_proxy web:3000
}
`,
		},
		{
			name: "service with invalid config is skipped",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"bad-service",
					"10.210.0.2",
					`# test:invalid
bad.config.com {
	respond "This config is invalid"
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase,
		},
		{
			name: "service with invalid config template is skipped",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"bad-template",
					"10.210.0.2",
					`
bad.template.com {
	reverse_proxy {{upstreams
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase,
		},
		{
			name: "caddy service with invalid global config is skipped",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.0.2",
					`# test:invalid
localhost {
	respond "Invalid global config"
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase,
		},
		{
			name: "caddy service on different machine is ignored",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.0.2",
					`# Global config from other machine
{
	global directive
}`,
					"other-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase,
		},
		{
			name: "multiple services with mixed valid and invalid configs",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"api",
					"10.210.0.2",
					`api.example.com {
	reverse_proxy api:8080
}`,
					"test-machine-id",
					time.Now(),
				),
				newContainerRecordWithCaddyConfig(
					"invalid-svc",
					"10.210.0.3",
					`# test:invalid
bad.example.com {
	respond "Invalid"
}`,
					"test-machine-id",
					time.Now(),
				),
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.0.4",
					`web.example.com {
	reverse_proxy web:3000
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase + `
# Service: api
api.example.com {
	reverse_proxy api:8080
}

# Service: web
web.example.com {
	reverse_proxy web:3000
}
`,
		},
		{
			name: "combined: caddy global config + service configs + ports",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.0.1",
					`# Global config
{
	global directive
}`,
					"test-machine-id",
					time.Now(),
				),
				newContainerRecordWithPorts(
					"app",
					"10.210.0.2",
					[]string{"app.example.com:8080/http"},
					"test-machine-id",
				),
				newContainerRecordWithCaddyConfig(
					"api",
					"10.210.0.3",
					`api.example.com {
	reverse_proxy api:8000
}`,
					"other-machine-id",
					time.Now(),
				),
			},
			want: `# Global config
{
	global directive
}

` + caddyfileBase + `
http://app.example.com {
	reverse_proxy 10.210.0.2:8080 {
		import common_proxy
	}
	log
}

# Service: api
api.example.com {
	reverse_proxy api:8000
}
`,
		},
		{
			name: "service with template directives using upstreams",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.0.2",
					`web.example.com {
	reverse_proxy {{upstreams}}
}`,
					"test-machine-id",
					time.Now(),
				),
				newContainerRecordWithPorts(
					"api",
					"10.210.0.3",
					[]string{"api.example.com:8080/http"},
					"test-machine-id",
				),
			},
			want: caddyfileBase + `
http://api.example.com {
	reverse_proxy 10.210.0.3:8080 {
		import common_proxy
	}
	log
}

# Service: web
web.example.com {
	reverse_proxy 10.210.0.2
}
`,
		},
		{
			name: "only most recent container config is used per service",
			containers: []store.ContainerRecord{
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.0.2",
					`# Old config
old.example.com {
	respond "Old"
}`,
					"test-machine-id",
					time.Now().Add(-1*time.Hour),
				),
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.0.3",
					`# New config
new.example.com {
	respond "New"
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: caddyfileBase + `
# Service: web
# New config
new.example.com {
	respond "New"
}
`,
		},
		{
			name: "compound test: upstreams variants, global caddy, and multi-machine services",
			containers: []store.ContainerRecord{
				// Global Caddy service on test-machine-id
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.1.1",
					`# Global config from test machine
{
	admin off
}

localhost:8080 {
	respond "Admin panel"
}`,
					"test-machine-id",
					time.Now(),
				),
				// Another caddy on different machine (should be ignored)
				newContainerRecordWithCaddyConfig(
					"caddy",
					"10.210.2.1",
					`# Should be ignored
{
	debug
}`,
					"machine-2",
					time.Now(),
				),

				// API service containers across different machines
				newContainerRecordWithPorts("api", "10.210.1.2", []string{"api.example.com:8080/http"},
					"test-machine-id"),
				newContainerRecordWithPorts("api", "10.210.2.2", []string{"api.example.com:8080/http"}, "machine-2"),
				newContainerRecordWithPorts("api", "10.210.3.2", []string{"api.example.com:8080/http"}, "machine-3"),

				// Web service with different versions on different machines
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.1.3",
					`# Web service config v1 (older)
web-v1.example.com {
	reverse_proxy web:3000
}`,
					"test-machine-id",
					time.Now().Add(-2*time.Hour),
				),
				newContainerRecordWithPorts("web", "10.210.3.3", []string{"web.example.com:3000/http"}, "machine-3"),
				newContainerRecordWithCaddyConfig(
					"web",
					"10.210.2.3",
					`# Web service config v2 (most recent)
web-v2.example.com {
	reverse_proxy {{upstreams 8080}}
}`,
					"machine-2",
					time.Now().Add(1*time.Second),
				),

				// DB service with custom config
				newContainerRecordWithCaddyConfig(
					"db",
					"10.210.1.4",
					`# DB admin panel
dbadmin.example.com {
	basicauth {
		admin $2a$14$Zkx19XLiW6VYouLHR5NmfOFU0z2GTNmpkT/5qqR7hx4IjWJPDhjvG
	}
	reverse_proxy {{upstreams 5432}}
}`,
					"test-machine-id",
					time.Now(),
				),

				// Gateway service with various upstream template usages
				newContainerRecordWithCaddyConfig(
					"gateway",
					"10.210.1.5",
					`# Testing different upstream template functions
gateway.example.com {
	# Current service upstreams (gateway)
	handle /self {
		reverse_proxy {{upstreams}}
	}

	# Named service upstreams without port
	handle /api {
		reverse_proxy {{upstreams "api"}}
	}

	# Named service upstreams with port
	handle /api-custom {
		reverse_proxy {{upstreams "api" 9000}}
	}

	# Current service with name and port
	handle /self-port {
		reverse_proxy {{upstreams .Name 8888}}
	}

	# Service with mixed containers (web) and advanced template
	handle /web {
		reverse_proxy {{- range $ip := index .Upstreams "web"}} https://{{$ip}}{{end}}
	}

	# Non-existent service
	handle /missing {
		reverse_proxy {{upstreams "nonexistent"}}
	}
}`,
					"test-machine-id",
					time.Now(),
				),

				// App service with just ports (no custom config)
				newContainerRecordWithPorts("app", "10.210.1.6", []string{"app.example.com:3000/http"},
					"test-machine-id"),
				newContainerRecordWithPorts("app", "10.210.2.6", []string{"app.example.com:3000/http"}, "machine-2"),

				// Service with invalid config (should be ignored)
				newContainerRecordWithCaddyConfig(
					"invalid",
					"10.210.1.7",
					`# test:invalid
badconfig.com {
	respond "This config is invalid"
}`,
					"test-machine-id",
					time.Now(),
				),
			},
			want: `# Global config from test machine
{
	admin off
}

localhost:8080 {
	respond "Admin panel"
}

` + caddyfileBase + `
http://api.example.com {
	reverse_proxy 10.210.1.2:8080 10.210.2.2:8080 10.210.3.2:8080 {
		import common_proxy
	}
	log
}

http://app.example.com {
	reverse_proxy 10.210.1.6:3000 10.210.2.6:3000 {
		import common_proxy
	}
	log
}

http://web.example.com {
	reverse_proxy 10.210.3.3:3000 {
		import common_proxy
	}
	log
}

# Service: db
# DB admin panel
dbadmin.example.com {
	basicauth {
		admin $2a$14$Zkx19XLiW6VYouLHR5NmfOFU0z2GTNmpkT/5qqR7hx4IjWJPDhjvG
	}
	reverse_proxy 10.210.1.4:5432
}

# Service: gateway
# Testing different upstream template functions
gateway.example.com {
	# Current service upstreams (gateway)
	handle /self {
		reverse_proxy 10.210.1.5
	}

	# Named service upstreams without port
	handle /api {
		reverse_proxy 10.210.1.2 10.210.2.2 10.210.3.2
	}

	# Named service upstreams with port
	handle /api-custom {
		reverse_proxy 10.210.1.2:9000 10.210.2.2:9000 10.210.3.2:9000
	}

	# Current service with name and port
	handle /self-port {
		reverse_proxy 10.210.1.5:8888
	}

	# Service with mixed containers (web) and advanced template
	handle /web {
		reverse_proxy https://10.210.1.3 https://10.210.3.3 https://10.210.2.3
	}

	# Non-existent service
	handle /missing {
` + "\t\treverse_proxy " + `
	}
}

# Service: web
# Web service config v2 (most recent)
web-v2.example.com {
	reverse_proxy 10.210.1.3:8080 10.210.3.3:8080 10.210.2.3:8080
}
`,
		},
	}

	ctx := context.Background()
	validator := NewMockCaddyfileValidator(t)
	validator.EXPECT().Validate(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, caddyfile string) error {
			if strings.Contains(caddyfile, "# test:invalid") {
				return errors.New("invalid config detected")
			}
			return nil
		})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := NewCaddyfileGenerator("test-machine-id", validator, nil)

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

func newContainerRecordWithCaddyConfig(serviceName, ip, caddyConfig, machineID string, created time.Time) store.ContainerRecord {
	return store.ContainerRecord{
		Container: api.ServiceContainer{
			Container: api.Container{
				ContainerJSON: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID: serviceName + "-" + ip, // Add ID for stable sorting
						State: &types.ContainerState{
							Running: true,
						},
						Created: created.UTC().Format(time.RFC3339Nano),
					},
					NetworkSettings: &types.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							docker.NetworkName: {
								IPAddress: ip,
							},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							api.LabelServiceName: serviceName,
						},
					},
				},
			},
			ServiceSpec: api.ServiceSpec{
				Caddy: &api.CaddySpec{
					Config: caddyConfig,
				},
			},
		},
		MachineID: machineID,
	}
}

func newContainerRecordWithPorts(serviceName, ip string, ports []string, machineID string) store.ContainerRecord {
	portsLabel := strings.Join(ports, ",")
	return store.ContainerRecord{
		Container: api.ServiceContainer{
			Container: api.Container{
				ContainerJSON: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID: serviceName + "-" + ip, // Add ID for stable sorting
						State: &types.ContainerState{
							Running: true,
						},
						Created: time.Now().UTC().Format(time.RFC3339Nano),
					},
					NetworkSettings: &types.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							docker.NetworkName: {
								IPAddress: ip,
							},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							api.LabelServiceName:  serviceName,
							api.LabelServicePorts: portsLabel,
						},
					},
				},
			},
		},
		MachineID: machineID,
	}
}
