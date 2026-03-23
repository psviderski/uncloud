package logs

import (
	"fmt"
	"image/color"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/spf13/pflag"
)

// Options describes how and what logs we are requesting.
type Options struct {
	Files    []string
	Follow   bool
	Tail     string
	Since    string
	Until    string
	UTC      bool
	Machines []string
}

func Flags(options *Options) *pflag.FlagSet {
	set := &pflag.FlagSet{}

	set.BoolVarP(&options.Follow, "follow", "f", false,
		"Continually stream new logs.")
	set.StringSliceVarP(&options.Machines, "machine", "m", nil,
		"Filter logs by machine name or ID. Can be specified multiple times or as a comma-separated list.")
	set.StringVar(&options.Since, "since", "",
		"Show logs generated on or after the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.\n"+
			"Examples:\n"+
			"  --since 2m30s                      Relative duration (2 minutes 30 seconds ago)\n"+
			"  --since 1h                         Relative duration (1 hour ago)\n"+
			"  --since 2025-11-24                 RFC 3339 date only (midnight using local timezone)\n"+
			"  --since 2024-05-14T22:50:00        RFC 3339 date/time using local timezone\n"+
			"  --since 2024-01-31T10:30:00Z       RFC 3339 date/time in UTC\n"+
			"  --since 1763953966                 Unix timestamp (seconds since January 1, 1970)")
	set.StringVarP(&options.Tail, "tail", "n", "100",
		"Show the most recent logs and limit the number of lines shown per replica. Use 'all' to show all logs.")
	set.StringVar(&options.Until, "until", "",
		"Show logs generated before the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.\n"+
			"See --since for examples.")
	set.BoolVar(&options.UTC, "utc", false,
		"Print timestamps in UTC instead of local timezone.")

	return set
}

func Tail(tail string) (int, error) {
	if tail == "all" {
		return -1, nil
	}

	tailInt, err := strconv.Atoi(tail)
	if err != nil {
		return 0, fmt.Errorf("invalid --tail value '%s': %w", tail, err)
	}
	return tailInt, nil
}

// Available colors for machine/service differentiation.
var Palette = []color.Color{
	lipgloss.BrightGreen,
	lipgloss.BrightYellow,
	lipgloss.BrightBlue,
	lipgloss.BrightMagenta,
	lipgloss.BrightCyan,
	lipgloss.Green,
	lipgloss.Yellow,
	lipgloss.Blue,
	lipgloss.Magenta,
	lipgloss.Cyan,
}
