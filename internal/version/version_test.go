package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModuleVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deps []*debug.Module
		want string
	}{
		{
			name: "uncloud is not a dependency",
			deps: []*debug.Module{{Path: "google.golang.org/grpc", Version: "v1.81.0"}},
			want: "",
		},
		{
			name: "release version",
			deps: []*debug.Module{
				{Path: "google.golang.org/grpc", Version: "v1.81.0"},
				{Path: modulePath, Version: "v0.22.0"},
			},
			want: "0.22.0",
		},
		{
			name: "pseudo-version",
			deps: []*debug.Module{{Path: modulePath, Version: "v0.22.1-0.20261117120000-abcdef123456"}},
			want: "0.22.1-0.20261117120000-abcdef123456",
		},
		{
			name: "replaced with another module version",
			deps: []*debug.Module{{
				Path:    modulePath,
				Version: "v0.22.0",
				Replace: &debug.Module{Path: "github.com/example/uncloud", Version: "v0.23.0"},
			}},
			want: "0.23.0",
		},
		{
			name: "replaced with local directory",
			deps: []*debug.Module{{
				Path:    modulePath,
				Version: "v0.22.0",
				Replace: &debug.Module{Path: "../uncloud", Version: "(devel)"},
			}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: tt.deps}
			assert.Equal(t, tt.want, moduleVersion(info))
		})
	}
}
