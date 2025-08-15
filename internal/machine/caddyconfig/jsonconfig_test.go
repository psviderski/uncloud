package caddyconfig

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateJSONConfig(t *testing.T) {
	configWithoutServices := `{
		"servers": {
			"http": {
				"listen": [":80"],
				"routes": [{
					"match": [{"path": ["/.uncloud-verify"]}],
					"handle": [{
						"body": "verification-response-body",
						"handler": "static_response",
						"status_code": 200
					}]
				}],
				"logs": {}
			},
			"https": {
				"listen": [":443"],
				"logs": {}
			}
		}
	}`

	tests := []struct {
		name       string
		containers []api.ServiceContainer
		want       string
		wantErr    bool
	}{
		{
			name:       "empty containers",
			containers: []api.ServiceContainer{},
			want:       configWithoutServices,
			wantErr:    false,
		},

		{
			name: "HTTP container",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
			},
			want: `{
                "servers": {
                    "http": {
                        "listen": [":80"],
                        "routes": [
                            {
                                "match": [{"host": ["app.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [{"dial": "10.210.0.2:8080"}]
                                }]
                            },
                            {
                                "match": [{"path": ["/.uncloud-verify"]}],
                                "handle": [{
                                    "body": "verification-response-body",
                                    "handler": "static_response",
                                    "status_code": 200
                                }]
                            }
                        ],
						"logs": {}
                    },
                    "https": {
                        "listen": [":443"],
						"logs": {}
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "load balancing multiple containers",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
				newContainer("10.210.0.3", "app.example.com:8080/http"),
			},
			want: `{
                "servers": {
                    "http": {
                        "listen": [":80"],
                        "routes": [
                            {
                                "match": [{"host": ["app.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [
                                        {"dial": "10.210.0.2:8080"},
                                        {"dial": "10.210.0.3:8080"}
                                    ]
                                }]
                            },
                            {
                                "match": [{"path": ["/.uncloud-verify"]}],
                                "handle": [{
                                    "body": "verification-response-body",
                                    "handler": "static_response",
                                    "status_code": 200
                                }]
                            }
                        ],
						"logs": {}
                    },
                    "https": {
                        "listen": [":443"],
						"logs": {}
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "HTTPS container",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "secure.example.com:8000/https"),
			},
			want: `{
                "servers": {
                    "http": {
                        "listen": [":80"],
                        "routes": [
                            {
                                "match": [{"path": ["/.uncloud-verify"]}],
                                "handle": [{
                                    "body": "verification-response-body",
                                    "handler": "static_response",
                                    "status_code": 200
                                }]
                            }
                        ],
						"logs": {}
                    },
                    "https": {
                        "listen": [":443"],
                        "routes": [
                            {
                                "match": [{"host": ["secure.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [{"dial": "10.210.0.2:8000"}]
                                }]
                            }
                        ],
						"logs": {}
                    }
                }
            }`,
			wantErr: false,
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
			want: `{
                "servers": {
                    "http": {
                        "listen": [":80"],
                        "routes": [
                            {
                                "match": [{"host": ["app.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [
										{"dial": "10.210.0.2:8080"},
										{"dial": "10.210.0.3:8080"},
										{"dial": "10.210.0.5:8080"}
									]
                                }]
                            },
							{
                                "match": [{"host": ["web.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [
										{"dial": "10.210.0.2:8000"},
										{"dial": "10.210.0.4:8000"},
										{"dial": "10.210.0.5:8000"}
									]
                                }]
                            },
                            {
                                "match": [{"path": ["/.uncloud-verify"]}],
                                "handle": [{
                                    "body": "verification-response-body",
                                    "handler": "static_response",
                                    "status_code": 200
                                }]
                            }
                        ],
						"logs": {}
                    },
                    "https": {
                        "listen": [":443"],
                        "routes": [
                            {
                                "match": [{"host": ["secure.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [
										{"dial": "10.210.0.3:8888"},
										{"dial": "10.210.0.4:8888"},
										{"dial": "10.210.0.5:8888"}
									]
                                }]
                            }
                        ],
						"logs": {}
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "container without uncloud network ignored",
			containers: []api.ServiceContainer{
				newContainerWithoutNetwork("ignored.example.com:8080/http"),
			},
			want:    configWithoutServices,
			wantErr: false,
		},
		{
			name: "container with invalid port ignored",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "invalid-port"),
			},
			want:    configWithoutServices,
			wantErr: false,
		},
		{
			name: "containers with unsupported protocols and host mode ignored",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "5000/tcp"),
				newContainer("10.210.0.3", "5000/udp"),
				newContainer("10.210.0.4", "80:8080/tcp@host"),
			},
			want:    configWithoutServices,
			wantErr: false,
		},
		{
			name: "restarting container ignored",
			containers: []api.ServiceContainer{
				newRestartingContainer("10.210.0.2", "app.example.com:8080/http"),
			},
			want:    configWithoutServices,
			wantErr: false,
		},
		{
			name: "stopped container ignored",
			containers: []api.ServiceContainer{
				newStoppedContainer("10.210.0.2", "app.example.com:8080/http"),
			},
			want:    configWithoutServices,
			wantErr: false,
		},
		{
			name: "mix of running, restarting, and stopped containers",
			containers: []api.ServiceContainer{
				newContainer("10.210.0.2", "app.example.com:8080/http"),
				newRestartingContainer("10.210.0.3", "app.example.com:8080/http"),
				newStoppedContainer("10.210.0.4", "app.example.com:8080/http"),
			},
			want: `{
                "servers": {
                    "http": {
                        "listen": [":80"],
                        "routes": [
                            {
                                "match": [{"host": ["app.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
									"health_checks": {
                                        "passive": {
                                            "fail_duration": 30000000000
                                        }
                                    },
                                    "load_balancing": {
                                        "retries": 3
                                    },
                                    "upstreams": [{"dial": "10.210.0.2:8080"}]
                                }]
                            },
                            {
                                "match": [{"path": ["/.uncloud-verify"]}],
                                "handle": [{
                                    "body": "verification-response-body",
                                    "handler": "static_response",
                                    "status_code": 200
                                }]
                            }
                        ],
						"logs": {}
                    },
                    "https": {
                        "listen": [":443"],
						"logs": {}
                    }
                }
            }`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := GenerateJSONConfig(tt.containers, "verification-response-body")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Len(t, config.AppsRaw, 1, "Expected one http app")
			require.Contains(t, config.AppsRaw, "http", "Expected http app")

			assert.JSONEq(t, tt.want, string(config.AppsRaw["http"]), "Generated Caddy app config doesn't match")
		})
	}
}

func newContainer(ip string, ports ...string) api.ServiceContainer {
	portsLabel := strings.Join(ports, ",")
	return api.ServiceContainer{Container: api.Container{ContainerJSON: types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{
				Running: true,
			},
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
				api.LabelServicePorts: portsLabel,
			},
		},
	}}}
}

func newContainerWithoutNetwork(ports ...string) api.ServiceContainer {
	portsLabel := strings.Join(ports, ",")
	return api.ServiceContainer{Container: api.Container{ContainerJSON: types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{
				Running: true,
			},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"other-network": {
					IPAddress: "172.17.0.2",
				},
			},
		},
		Config: &container.Config{
			Labels: map[string]string{
				api.LabelServicePorts: portsLabel,
			},
		},
	}}}
}

func newRestartingContainer(ip string, ports ...string) api.ServiceContainer {
	ctr := newContainer(ip, ports...)
	ctr.Container.State.Restarting = true
	return ctr
}

func newStoppedContainer(ip string, ports ...string) api.ServiceContainer {
	ctr := newContainer(ip, ports...)
	ctr.Container.State.Running = false
	return ctr
}
