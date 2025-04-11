package cli

import (
	"strings"
)

// ExpandCommaSeparatedValues takes a slice of strings and expands any comma-separated values into individual elements.
func ExpandCommaSeparatedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	var expanded []string
	for _, value := range values {
		for _, v := range strings.Split(value, ",") {
			if v = strings.TrimSpace(v); v != "" {
				expanded = append(expanded, v)
			}
		}
	}

	return expanded
}
