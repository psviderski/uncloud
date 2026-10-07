package logs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TimeRange resolves log filters that could be RFC 3339, timestamp, or relative duration to UTC timestamps.
// Dates without a timezone use now's location. Relative durations are computed from now.
func TimeRange(since, until string, now time.Time) (string, string, error) {
	var err error
	since, err = timestamp(since, now)
	if err != nil {
		return "", "", fmt.Errorf("invalid --since value: %w", err)
	}
	until, err = timestamp(until, now)
	if err != nil {
		return "", "", fmt.Errorf("invalid --until value: %w", err)
	}

	return since, until, nil
}

func timestamp(value string, now time.Time) (string, error) {
	if value == "" {
		return "", nil
	}
	// A bare zero is the Unix epoch, matching Docker's log filters.
	if duration, err := ParseDuration(value); value != "0" && err == nil {
		return now.Add(-duration).UTC().Format(time.RFC3339Nano), nil
	}

	// Keep Docker's supported date layouts, but use the location's offset at the requested date.
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15Z07:00",
		"2006-01-02Z07:00",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04",
		"2006-01-02T15",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, value, now.Location()); err == nil {
			return t.UTC().Format(time.RFC3339Nano), nil
		}
	}

	seconds, fraction, hasFraction := strings.Cut(value, ".")
	sec, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse '%s' as a time or duration", value)
	}

	var nsec int64
	if hasFraction {
		if len(fraction) == 0 || len(fraction) > 9 || strings.ContainsAny(fraction, "+-") {
			return "", fmt.Errorf("invalid Unix timestamp fraction in '%s'", value)
		}
		nsec, err = strconv.ParseInt(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid Unix timestamp fraction in '%s': %w", value, err)
		}
	}
	t := time.Unix(sec, nsec).UTC()
	if t.Year() < 0 || t.Year() > 9999 {
		return "", fmt.Errorf("Unix timestamp '%s' is outside the RFC 3339 date range", value)
	}
	return t.Format(time.RFC3339Nano), nil
}
