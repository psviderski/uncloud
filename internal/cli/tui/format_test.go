package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatRTT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "0ms"},
		{"rounds down", 39*time.Millisecond + 200*time.Microsecond, "39ms"},
		{"rounds up at half", 39*time.Millisecond + 500*time.Microsecond, "40ms"},
		{"rounds up near next ms", 39*time.Millisecond + 600*time.Microsecond, "40ms"},
		{"exact ms", 140 * time.Millisecond, "140ms"},
		{"one second", time.Second, "1000ms"},
		{"1.5s", 1500 * time.Millisecond, "1500ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatRTT(tt.in))
		})
	}
}
