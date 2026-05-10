package logs

import (
	"fmt"
	"strconv"
	"strings"

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

// ServiceArg pairs a service name with the list of container filters parsed from `uc service logs` arguments.
// An empty Containers slice means "stream logs from all containers of this service".
type ServiceArg struct {
	Service    string
	Containers []string
}

// ParseServiceArgs groups raw `uc service logs` positional arguments into per-service entries,
// preserving first-seen order. Each argument is either a service name (e.g. "web") or a
// service/container reference (e.g. "web/61d57fd3428f").
// Arguments for the same service are merged: if any argument for a service lacks a container suffix, all containers
// of that service are streamed regardless of any other service/container arguments for it.
func ParseServiceArgs(args []string) ([]ServiceArg, error) {
	indexByService := make(map[string]int, len(args))
	allContainers := make(map[string]bool, len(args))
	result := make([]ServiceArg, 0, len(args))

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			return nil, fmt.Errorf("empty service argument")
		}

		service, container, hasSlash := strings.Cut(arg, "/")
		if service == "" {
			return nil, fmt.Errorf("invalid service argument '%s': service name is empty", arg)
		}
		if hasSlash && container == "" {
			return nil, fmt.Errorf("invalid service argument '%s': container name or ID is empty", arg)
		}

		idx, seen := indexByService[service]
		if !seen {
			entry := ServiceArg{Service: service}
			if hasSlash {
				entry.Containers = []string{container}
			} else {
				allContainers[service] = true
			}
			result = append(result, entry)
			indexByService[service] = len(result) - 1
			continue
		}

		if !hasSlash {
			result[idx].Containers = nil
			allContainers[service] = true
			continue
		}
		if !allContainers[service] {
			result[idx].Containers = append(result[idx].Containers, container)
		}
	}

	return result, nil
}
