package caddyfile

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"github.com/psviderski/uncloud/internal/api"
	"github.com/psviderski/uncloud/internal/machine/docker"
)

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name       string
		containers []api.Container
		want       string
		wantErr    bool
	}{
		{
			name:       "empty containers",
			containers: []api.Container{},
			want: `{
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
                        }]
                    },
                    "https": {
                        "listen": [":443"]
                    }
				}
			}`,
			wantErr: false,
		},

		{
			name: "HTTP container",
			containers: []api.Container{
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
                        ]
                    },
                    "https": {
                        "listen": [":443"]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "load balancing multiple containers",
			containers: []api.Container{
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
                        ]
                    },
                    "https": {
                        "listen": [":443"]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "HTTPS container",
			containers: []api.Container{
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
                        ]
                    },
                    "https": {
                        "listen": [":443"],
                        "routes": [
                            {
                                "match": [{"host": ["secure.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
                                    "upstreams": [{"dial": "10.210.0.2:8000"}]
                                }]
                            }
                        ]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "mixed HTTP and HTTPS",
			containers: []api.Container{
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
                        ]
                    },
                    "https": {
                        "listen": [":443"],
                        "routes": [
                            {
                                "match": [{"host": ["secure.example.com"]}],
                                "handle": [{
                                    "handler": "reverse_proxy",
                                    "upstreams": [
										{"dial": "10.210.0.3:8888"},
										{"dial": "10.210.0.4:8888"},
										{"dial": "10.210.0.5:8888"}
									]
                                }]
                            }
                        ]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "container without uncloud network ignored",
			containers: []api.Container{
				newContainerWithoutNetwork("ignored.example.com:8080/http"),
			},
			want: `{
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
                        }]
                    },
                    "https": {
                        "listen": [":443"]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "container with invalid port ignored",
			containers: []api.Container{
				newContainer("10.210.0.2", "invalid-port"),
			},
			want: `{
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
                        }]
                    },
                    "https": {
                        "listen": [":443"]
                    }
                }
            }`,
			wantErr: false,
		},
		{
			name: "containers with unsupported protocols ignored",
			containers: []api.Container{
				newContainer("10.210.0.2", "5000/tcp"),
				newContainer("10.210.0.3", "5000/udp"),
				newContainer("10.210.0.4", "80:8080/tcp@host"),
			},
			want: `{
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
                        }]
                    },
                    "https": {
                        "listen": [":443"]
                    }
                }
            }`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := GenerateConfig(tt.containers, "verification-response-body")

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

func newContainer(ip string, ports ...string) api.Container {
	portsLabel := strings.Join(ports, ",")
	return api.Container{
		ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{},
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
		},
	}
}

func newContainerWithoutNetwork(ports ...string) api.Container {
	portsLabel := strings.Join(ports, ",")
	return api.Container{
		ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{},
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
		},
	}
}
