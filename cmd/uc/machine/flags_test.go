package machine

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runVersionFlagEnvTest builds the given command for each case, stubs out RunE so only flag
// parsing and PreRunE run (no SSH calls), and asserts the resolved --version flag value.
// Note: cannot use t.Parallel() because subtests use t.Setenv().
func runVersionFlagEnvTest(t *testing.T, newCommand func() *cobra.Command) {
	t.Helper()

	tests := []struct {
		name string
		args []string
		env  string
		want string
	}{
		{
			name: "env var sets version when flag not passed",
			args: []string{"root@localhost"},
			env:  "v1.2.3",
			want: "v1.2.3",
		},
		{
			name: "explicit flag wins over env var",
			args: []string{"root@localhost", "--version", "v9.9.9"},
			env:  "v1.2.3",
			want: "v9.9.9",
		},
		{
			name: "defaults to latest when neither flag nor env var set",
			args: []string{"root@localhost"},
			want: "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("UNCLOUD_DAEMON_VERSION", tt.env)
			}

			cmd := newCommand()
			cmd.RunE = func(*cobra.Command, []string) error {
				return nil
			}
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())

			version, err := cmd.Flags().GetString("version")
			require.NoError(t, err)
			assert.Equal(t, tt.want, version)
		})
	}
}

// TestInitCommandVersionFlagEnvVar verifies that the UNCLOUD_DAEMON_VERSION environment variable
// is bound to the --version flag of 'machine init', and that an explicit flag wins.
func TestInitCommandVersionFlagEnvVar(t *testing.T) {
	runVersionFlagEnvTest(t, NewInitCommand)
}

// TestAddCommandVersionFlagEnvVar verifies that the UNCLOUD_DAEMON_VERSION environment variable
// is bound to the --version flag of 'machine add', and that an explicit flag wins.
func TestAddCommandVersionFlagEnvVar(t *testing.T) {
	runVersionFlagEnvTest(t, NewAddCommand)
}
