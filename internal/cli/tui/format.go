package tui

import (
	"fmt"
	"math"
	"time"
)

// FormatRTT formats a round-trip time duration as whole milliseconds, e.g. "140ms".
func FormatRTT(d time.Duration) string {
	return fmt.Sprintf("%dms", int64(math.Round(float64(d)/float64(time.Millisecond))))
}
