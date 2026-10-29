package logs

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		input string
		want  time.Duration
	}{
		{"0", 0},
		{"2m30s", 150 * time.Second},
		{"1µs", time.Microsecond},
		{"1μs", time.Microsecond},
		{"2d", 48 * time.Hour},
		{"010d", 240 * time.Hour},
		{"08d", 192 * time.Hour},
		{"2d3h", 51 * time.Hour},
		{"3h2d", 51 * time.Hour},
		{"1d1d", 48 * time.Hour},
		{"1.5d", 36 * time.Hour},
		{".5d", 12 * time.Hour},
		{"1.d", 24 * time.Hour},
		{"-2d3h", -51 * time.Hour},
		{"+2d", 48 * time.Hour},
		{"0d", 0},
		{"0d1ns", time.Nanosecond},
		{"0.000000000001d", 86 * time.Nanosecond},
		{"-0.000000000001d", -86 * time.Nanosecond},
		{"106751d23h47m16.854775807s", time.Duration(math.MaxInt64)},
		{"-106751d23h47m16.854775808s", time.Duration(math.MinInt64)},
	} {
		t.Run(tt.input, func(t *testing.T) {
			actual, err := ParseDuration(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"", "d", "2d3", "1..2d", ".d", "1d-2h", "1d+2h", "1d 2h", "1day", "2D", "1w",
		"1e2d", "1d2w", "106752d", "-106752d", "106751d23h47m16.854775808s",
		"-106751d23h47m16.854775809s", "999999999999999999999999999d",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseDuration(input)
			require.Error(t, err)
		})
	}
}
