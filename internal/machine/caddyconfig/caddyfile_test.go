package caddyconfig

import (
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCaddyfile(t *testing.T) {
	caddyfileHeader := `http:// {
	handle /.uncloud-verify {
		respond "verification-response-body" 200
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
		containers []api.ServiceContainer
		want       string
		wantErr    bool
	}{
		{
			name:       "empty containers",
			containers: []api.ServiceContainer{},
			want:       caddyfileHeader,
		},
		{
			name: "HTTP container",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
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
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
				newContainer("10.210.0.3", "app.example.com:8080/http"),
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
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "secure.example.com:8000/https"),
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
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2",
					"app.example.com:8080/http",
					"web.example.com:8000/http"),
				newContainer("10.210.0.3",
					"app.example.com:8080/http",
					"secure.example.com:8888/https"),
				newContainer("10.210.0.4",
					"web.example.com:8000/http",
					"secure.example.com:8888/https"),
				newContainer("10.210.0.5",
					"app.example.com:8080/http",
					"web.example.com:8000/http",
					"secure.example.com:8888/https"),
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
			containers: []api.ServiceContainer{
				newContainerWithoutNetwork("ignored.example.com:8080/http"),
			},
			want: caddyfileHeader,
		},
		{
			name: "container with invalid port ignored",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "invalid-port"),
			},
			want: caddyfileHeader,
		},
		{
			name: "containers with unsupported protocols and host mode ignored",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "5000/tcp"),
				newContainer("10.210.0.3", "5000/udp"),
				newContainer("10.210.0.4", "80:8080/tcp@host"),
			},
			want: caddyfileHeader,
		},
		{
			name: "restarting container ignored",
			containers: []api.ServiceContainer{
				newRestartingContainer("10.210.0.2", "app.example.com:8080/http"),
			},
			want: caddyfileHeader,
		},
		{
			name: "stopped container ignored",
			containers: []api.ServiceContainer{
				newStoppedContainer("10.210.0.2", "app.example.com:8080/http"),
			},
			want: caddyfileHeader,
		},
		{
			name: "mix of running, restarting, and stopped containers",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
				newRestartingContainer("10.210.0.3", "app.example.com:8080/http"),
				newStoppedContainer("10.210.0.4", "app.example.com:8080/http"),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := GenerateCaddyfile(tt.containers, "verification-response-body")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.want, config, "Generated Caddyfile doesn't match")
		})
	}
}
