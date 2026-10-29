package logs

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Duration is an extended standard time.Duration that also supports days as a unit.
type Duration = time.Duration

// ParseDuration parses a Go duration with the additional unit d, meaning exactly 24 hours.
// Days can be fractional or combined with other units, such as "1.5d" or "2d3h".
func ParseDuration(value string) (Duration, error) {
	if !strings.Contains(value, "d") {
		return time.ParseDuration(value)
	}

	rest := value
	var normalised strings.Builder
	if len(rest) > 0 && (rest[0] == '-' || rest[0] == '+') {
		normalised.WriteByte(rest[0])
		rest = rest[1:]
	}
	for len(rest) > 0 {
		// Each component is a decimal number followed by a unit. Only the leading sign is allowed.
		numberEnd := 0
		for numberEnd < len(rest) && (isDigit(rest[numberEnd]) || rest[numberEnd] == '.') {
			numberEnd++
		}
		unitEnd := numberEnd
		for unitEnd < len(rest) && !isDigit(rest[unitEnd]) && rest[unitEnd] != '.' {
			unitEnd++
		}
		if numberEnd == 0 || unitEnd == numberEnd {
			return 0, fmt.Errorf("time: invalid duration '%s'", value)
		}
		number, unit := rest[:numberEnd], rest[numberEnd:unitEnd]
		if unit == "d" {
			// Use exact decimal arithmetic to avoid float rounding and retain nanosecond precision.
			days, ok := new(big.Rat).SetString(number)
			if !ok {
				return 0, fmt.Errorf("time: invalid duration '%s'", value)
			}
			days.Mul(days, new(big.Rat).SetInt64(int64(24*time.Hour)))
			nanoseconds := new(big.Int).Quo(days.Num(), days.Denom())
			normalised.WriteString(nanoseconds.String())
			normalised.WriteString("ns")
		} else {
			normalised.WriteString(rest[:unitEnd])
		}
		rest = rest[unitEnd:]
	}

	// The standard parser validates the remaining units and checks the total for overflow.
	duration, err := time.ParseDuration(normalised.String())
	if err != nil {
		return 0, fmt.Errorf("time: invalid duration '%s': %w", value, err)
	}
	return duration, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
