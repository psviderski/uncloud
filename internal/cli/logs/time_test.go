package logs

import (
	"testing"
	"time"

	timetypes "github.com/docker/docker/api/types/time"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeRange(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Australia/Sydney")
	require.NoError(t, err)
	// January is daylight-saving time, but July timestamps must use the winter offset.
	now := time.Date(2026, 1, 15, 12, 0, 0, 123456789, location)
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"2026-07-01", "2026-06-30T14:00:00Z"},
		{"2026-07-01T10", "2026-07-01T00:00:00Z"},
		{"2026-07-01T10:30", "2026-07-01T00:30:00Z"},
		{"2026-07-01T10:30:45.123456789", "2026-07-01T00:30:45.123456789Z"},
		{"2026-01-01T10:00:00", "2025-12-31T23:00:00Z"},
		{"2026-07-01T10:30:45Z", "2026-07-01T10:30:45Z"},
		{"2026-07-01T10:30:45+02:00", "2026-07-01T08:30:45Z"},
		{"2026-07-01T10:30:45-04:00", "2026-07-01T14:30:45Z"},
		{"2026-07-01T10+02:00", "2026-07-01T08:00:00Z"},
		{"2026-07-01T10:30+02:00", "2026-07-01T08:30:00Z"},
		{"2026-07-01+02:00", "2026-06-30T22:00:00Z"},
		{"1763953966", "2025-11-24T03:12:46Z"},
		{"1763953966.000000001", "2025-11-24T03:12:46.000000001Z"},
		{"0", "1970-01-01T00:00:00Z"},
		{"2m30s", "2026-01-15T00:57:30.123456789Z"},
		{"-1h", "2026-01-15T02:00:00.123456789Z"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			since, until, err := TimeRange(tt.input, tt.input, now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, since)
			assert.Equal(t, tt.want, until)

			if since != "" {
				// The server-side Docker SDK must preserve the cutoff even in another timezone.
				actual, err := timetypes.GetTimestamp(since, now.In(time.UTC))
				require.NoError(t, err)
				expected, err := time.Parse(time.RFC3339Nano, tt.want)
				require.NoError(t, err)
				sec, nsec, err := timetypes.ParseTimestamps(actual, 0)
				require.NoError(t, err)
				assert.True(t, expected.Equal(time.Unix(sec, nsec)))
			}
		})
	}

	since, until, err := TimeRange("3h", "1h30m", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-14T22:00:00.123456789Z", since)
	assert.Equal(t, "2026-01-14T23:30:00.123456789Z", until)
}

func TestTimeRange_Invalid(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"invalid", "2026-02-30", "2026-01-01T25:00:00", "1763953966.xyz",
		"1763953966.1234567890", "253402300800",
	} {
		t.Run(input, func(t *testing.T) {
			_, _, err := TimeRange(input, "", time.Now())
			require.ErrorContains(t, err, "invalid --since value")
			_, _, err = TimeRange("", input, time.Now())
			require.ErrorContains(t, err, "invalid --until value")
		})
	}
}
