package deploy

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/operation"
	"github.com/stretchr/testify/assert"
)

func TestServicePlan_FormatSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ops      []operation.Operation
		expected string
	}{
		{
			name: "in-place updates only",
			ops: []operation.Operation{
				&operation.UpdateContainerOperation{MachineID: "m-1"},
				&operation.UpdateContainerOperation{MachineID: "m-2"},
			},
			expected: "2 update (in place) · across 2 machines",
		},
		{
			name: "stop operations not counted as they are followed by remove",
			ops: []operation.Operation{
				&operation.StopContainerOperation{MachineID: "m-1"},
				&operation.StopContainerOperation{MachineID: "m-1"},
				&operation.ReplaceContainerOperation{MachineID: "m-1", Order: api.UpdateOrderStopFirst},
				&operation.RemoveContainerOperation{MachineID: "m-1"},
			},
			expected: "1 replace (stop-first) · 1 remove · across 1 machine",
		},
		{
			name: "all operation types",
			ops: []operation.Operation{
				&operation.RunContainerOperation{MachineID: "m-1"},
				&operation.ReplaceContainerOperation{MachineID: "m-1", Order: api.UpdateOrderStartFirst},
				&operation.ReplaceContainerOperation{MachineID: "m-2", Order: api.UpdateOrderStopFirst},
				&operation.UpdateContainerOperation{MachineID: "m-2"},
				&operation.RemoveContainerOperation{MachineID: "m-3"},
			},
			expected: "1 create · 1 replace (start-first) · 1 replace (stop-first) · 1 update (in place) · 1 remove · " +
				"across 3 machines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := ServicePlan{}
			plan.Operations = tt.ops
			assert.Equal(t, tt.expected, ansi.Strip(plan.FormatSummary()))
		})
	}
}

func TestServicePlan_Format(t *testing.T) {
	t.Parallel()

	newContainer := func(spec api.ServiceSpec) api.ServiceContainer {
		return api.ServiceContainer{
			Container: api.Container{InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{ID: "0f6df6593e1b0123456789abcdef"},
			}},
			ServiceSpec: spec,
		}
	}
	spec := func(image string, labels, containerLabels map[string]string) api.ServiceSpec {
		return api.ServiceSpec{
			Name:      "web",
			Mode:      api.ServiceModeReplicated,
			Replicas:  1,
			Labels:    labels,
			Container: api.ContainerSpec{Image: image, Labels: containerLabels},
		}
	}

	withCaddy := func(config string) api.ServiceSpec {
		s := spec("nginx:1.25", nil, nil)
		s.Caddy = &api.CaddySpec{Config: config}
		return s
	}
	replaceFrom := func(old api.ServiceSpec) func(api.ServiceSpec) []operation.Operation {
		return func(new api.ServiceSpec) []operation.Operation {
			return []operation.Operation{&operation.ReplaceContainerOperation{
				Spec:         new,
				MachineName:  "machine-1",
				OldContainer: newContainer(old),
			}}
		}
	}

	tests := []struct {
		name         string
		isNewService bool
		spec         api.ServiceSpec
		ops          func(spec api.ServiceSpec) []operation.Operation
		expected     []string
	}{
		{
			name: "caddy config lines added and removed",
			spec: withCaddy("{\n\tstorage uncloud\n}"),
			ops:  replaceFrom(withCaddy("{\n\tdebug\n\tauto_https off\n}")),
			expected: []string{
				"~ update service web",
				"  │   image:        nginx:1.25",
				"  │ ~ caddy config: +1 -2 lines",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "caddy config single line added",
			spec: withCaddy("{\n\tstorage uncloud\n\tdebug\n}"),
			ops:  replaceFrom(withCaddy("{\n\tstorage uncloud\n}")),
			expected: []string{
				"~ update service web",
				"  │   image:        nginx:1.25",
				"  │ ~ caddy config: +1 line",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "caddy config added to existing service",
			spec: withCaddy("{\n\tstorage uncloud\n}"),
			ops:  replaceFrom(spec("nginx:1.25", nil, nil)),
			expected: []string{
				"~ update service web",
				"  │   image:        nginx:1.25",
				"  │ + caddy config: +3 lines",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "caddy config removed",
			spec: spec("nginx:1.25", nil, nil),
			ops:  replaceFrom(withCaddy("{\n\tstorage uncloud\n}")),
			expected: []string{
				"~ update service web",
				"  │   image:        nginx:1.25",
				"  │ - caddy config: -3 lines",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "caddy config with only reordered lines",
			spec: withCaddy("a {\n}\nb {\n}"),
			ops:  replaceFrom(withCaddy("b {\n}\na {\n}")),
			expected: []string{
				"~ update service web",
				"  │   image:        nginx:1.25",
				"  │ ~ caddy config: changed",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "caddy config unchanged ignoring surrounding whitespace",
			spec: withCaddy("{\n\tdebug\n}"),
			ops:  replaceFrom(withCaddy("\n{\n\tdebug\n}\n")),
			expected: []string{
				"~ update service web",
				"  │   image: nginx:1.25",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name:         "new service with caddy config",
			isNewService: true,
			spec:         withCaddy("{\n\tstorage uncloud\n}"),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.RunContainerOperation{Spec: new, MachineName: "machine-1"}}
			},
			expected: []string{
				"+ create service web",
				"  │   image:        nginx:1.25",
				"  │   caddy config: +3 lines",
				"  │",
				"  ╰── +   run container web on machine-1",
			},
		},
		{
			name:         "caddy config before replicas",
			isNewService: true,
			spec: func() api.ServiceSpec {
				s := withCaddy("{\n\tstorage uncloud\n}")
				s.Replicas = 3
				return s
			}(),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.RunContainerOperation{Spec: new, MachineName: "machine-1"}}
			},
			expected: []string{
				"+ create service web",
				"  │   image:        nginx:1.25",
				"  │   caddy config: +3 lines",
				"  │   replicas:     3",
				"  │",
				"  ╰── +   run container web on machine-1",
			},
		},
		{
			name: "service labels added, changed, and removed in place",
			spec: spec("nginx:1.25", map[string]string{"env": "prod", "team": "platform", "same": "1"}, nil),
			ops: func(api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.UpdateContainerOperation{
					MachineName: "machine-1",
					Container: newContainer(spec(
						"nginx:1.25", map[string]string{"old": "legacy", "team": "backend", "same": "1"}, nil,
					)),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │   image: nginx:1.25",
				"  │ ~ labels:",
				"  │     + env=prod",
				"  │     - old=legacy",
				"  │     ~ team=backend → platform",
				"  │",
				"  ╰── ~   update container web/0f6df6593e1b on machine-1 (in place)",
			},
		},
		{
			name: "container labels changed with image on replace",
			spec: spec("nginx:1.26", nil, map[string]string{"proxy": "on"}),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.ReplaceContainerOperation{
					Spec:         new,
					MachineName:  "machine-1",
					OldContainer: newContainer(spec("nginx:1.25", nil, map[string]string{"proxy": "off"})),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │ ~ image: nginx:1.25 → 1.26",
				"  │ ~ container labels:",
				"  │     ~ proxy=off → on",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "no label blocks when labels are the same",
			spec: spec("nginx:1.26", map[string]string{}, map[string]string{"proxy": "on"}),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.ReplaceContainerOperation{
					Spec:         new,
					MachineName:  "machine-1",
					OldContainer: newContainer(spec("nginx:1.25", nil, map[string]string{"proxy": "on"})),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │ ~ image: nginx:1.25 → 1.26",
				"  │",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-1",
			},
		},
		{
			name: "replaced container determines old spec over updated one",
			spec: spec("nginx:1.26", map[string]string{"team": "platform"}, nil),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{
					&operation.UpdateContainerOperation{
						MachineName: "machine-1",
						Container:   newContainer(spec("nginx:1.26", map[string]string{"team": "backend"}, nil)),
					},
					&operation.ReplaceContainerOperation{
						Spec:         new,
						MachineName:  "machine-2",
						OldContainer: newContainer(spec("nginx:1.25", map[string]string{"team": "backend"}, nil)),
					},
				}
			},
			expected: []string{
				"~ update service web",
				"  │ ~ image: nginx:1.25 → 1.26",
				"  │ ~ labels:",
				"  │     ~ team=backend → platform",
				"  │",
				"  ├── ~   update container web/0f6df6593e1b on machine-1 (in place)",
				"  ╰── +/- replace container web/0f6df6593e1b on machine-2",
			},
		},
		{
			name: "all labels removed",
			spec: spec("nginx:1.25", map[string]string{}, nil),
			ops: func(api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.UpdateContainerOperation{
					MachineName: "machine-1",
					Container:   newContainer(spec("nginx:1.25", map[string]string{"a": "1", "b": "2"}, nil)),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │   image: nginx:1.25",
				"  │ - labels:",
				"  │     - a=1",
				"  │     - b=2",
				"  │",
				"  ╰── ~   update container web/0f6df6593e1b on machine-1 (in place)",
			},
		},
		{
			name: "first labels added to existing service",
			spec: spec("nginx:1.25", map[string]string{"a": "1"}, nil),
			ops: func(api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.UpdateContainerOperation{
					MachineName: "machine-1",
					Container:   newContainer(spec("nginx:1.25", nil, nil)),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │   image: nginx:1.25",
				"  │ + labels:",
				"  │     + a=1",
				"  │",
				"  ╰── ~   update container web/0f6df6593e1b on machine-1 (in place)",
			},
		},
		{
			name: "empty and non-printable label values are escaped on a single line",
			spec: spec("nginx:1.25", map[string]string{
				"added":    "1\n2",
				"cleared":  "",
				"json":     `{"a":1}`,
				"multi":    "a\nb\nc",
				"tab":      "a\tb",
				"trailing": "hello\n",
				"unicode":  "héllo wörld 🚀",
			}, nil),
			ops: func(api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.UpdateContainerOperation{
					MachineName: "machine-1",
					Container: newContainer(spec("nginx:1.25", map[string]string{
						"cleared":  "x",
						"multi":    "a\nb",
						"trailing": "hello",
					}, nil)),
				}}
			},
			expected: []string{
				"~ update service web",
				"  │   image: nginx:1.25",
				"  │ ~ labels:",
				`  │     + added="1\n2"`,
				`  │     ~ cleared=x → ""`,
				`  │     + json={"a":1}`,
				`  │     ~ multi="a\nb" → "a\nb\nc"`,
				`  │     + tab="a\tb"`,
				`  │     ~ trailing=hello → "hello\n"`,
				"  │     + unicode=héllo wörld 🚀",
				"  │",
				"  ╰── ~   update container web/0f6df6593e1b on machine-1 (in place)",
			},
		},
		{
			name:         "new service with labels",
			isNewService: true,
			spec:         spec("nginx:1.25", map[string]string{"team": "platform"}, map[string]string{"proxy": "on"}),
			ops: func(new api.ServiceSpec) []operation.Operation {
				return []operation.Operation{&operation.RunContainerOperation{Spec: new, MachineName: "machine-1"}}
			},
			expected: []string{
				"+ create service web",
				"  │   image: nginx:1.25",
				"  │   labels:",
				"  │       team=platform",
				"  │   container labels:",
				"  │       proxy=on",
				"  │",
				"  ╰── +   run container web on machine-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := ServicePlan{ServiceName: "web", IsNewService: tt.isNewService, Spec: tt.spec}
			plan.Operations = tt.ops(tt.spec)

			// The table pads cells to the column width so ignore trailing whitespace.
			var lines []string
			for line := range strings.Lines(ansi.Strip(plan.Format())) {
				lines = append(lines, strings.TrimRight(line, " \n"))
			}
			assert.Equal(t, tt.expected, lines)
		})
	}
}
