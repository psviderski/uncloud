package corrosion

import (
	"math"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeRTTStatsMs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		samples    []float64
		wantMedian float64
		wantStdDev float64
	}{
		{
			name:       "single sample",
			samples:    []float64{42},
			wantMedian: 42,
			wantStdDev: 0,
		},
		{
			name:       "two samples (even, averaged)",
			samples:    []float64{10, 20},
			wantMedian: 15,
			// Population stddev: variance = ((10-15)^2 + (20-15)^2)/2 = 25; sqrt = 5.
			wantStdDev: 5,
		},
		{
			name:       "three samples (odd, middle)",
			samples:    []float64{3, 1, 2},
			wantMedian: 2,
			// Mean = 2; variance = (1+0+1)/3 = 0.666...; stddev = sqrt(2/3).
			wantStdDev: math.Sqrt(2.0 / 3.0),
		},
		{
			name:       "four samples (even, averaged)",
			samples:    []float64{1, 2, 3, 4},
			wantMedian: 2.5,
			// Mean = 2.5; variance = (2.25+0.25+0.25+2.25)/4 = 1.25; stddev = sqrt(1.25).
			wantStdDev: math.Sqrt(1.25),
		},
		{
			// Verifies that unsorted input is sorted before picking the median.
			name:       "unsorted input",
			samples:    []float64{100, 1, 50},
			wantMedian: 50,
			// Mean = 151/3. Variance = ((149^2 + 148^2 + 1^2) / 9) / 3.
			wantStdDev: math.Sqrt(float64(149*149+148*148+1) / 9.0 / 3.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			median, stdDev := computeRTTStatsMs(tt.samples)
			assert.InDelta(t, tt.wantMedian, median, 1e-9, "median")
			assert.InDelta(t, tt.wantStdDev, stdDev, 1e-9, "stdDev")
		})
	}
}

func TestParseClusterMemberRTT(t *testing.T) {
	validState := map[string]any{"addr": "[fdcc:b618:5034:7afa:172a:1452:f2de:3c99]:51001"}

	tests := []struct {
		name      string
		input     map[string]any
		wantAddr  string
		wantRTTs  []float64
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid state and rtts",
			input: map[string]any{
				"state": validState,
				"rtts":  []any{float64(10), float64(20), float64(30)},
			},
			wantAddr: "[fdcc:b618:5034:7afa:172a:1452:f2de:3c99]:51001",
			wantRTTs: []float64{10, 20, 30},
		},
		{
			name: "missing rtts key is not an error",
			input: map[string]any{
				"state": validState,
			},
			wantAddr: "[fdcc:b618:5034:7afa:172a:1452:f2de:3c99]:51001",
			wantRTTs: nil,
		},
		{
			name: "null rtts value is not an error",
			input: map[string]any{
				"state": validState,
				"rtts":  nil,
			},
			wantAddr: "[fdcc:b618:5034:7afa:172a:1452:f2de:3c99]:51001",
			wantRTTs: nil,
		},
		{
			name: "empty rtts array is not an error",
			input: map[string]any{
				"state": validState,
				"rtts":  []any{},
			},
			wantAddr: "[fdcc:b618:5034:7afa:172a:1452:f2de:3c99]:51001",
			wantRTTs: nil,
		},
		{
			name: "non-array rtts",
			input: map[string]any{
				"state": validState,
				"rtts":  "not-an-array",
			},
			wantErr:   true,
			errSubstr: "invalid 'rtts' field type",
		},
		{
			name: "non-number element in rtts",
			input: map[string]any{
				"state": validState,
				"rtts":  []any{float64(10), "bad"},
			},
			wantErr:   true,
			errSubstr: "invalid rtt value type",
		},
		{
			name:      "missing state",
			input:     map[string]any{"rtts": []any{float64(1)}},
			wantErr:   true,
			errSubstr: "missing or invalid 'state' field",
		},
		{
			name: "missing addr in state",
			input: map[string]any{
				"state": map[string]any{},
				"rtts":  []any{float64(1)},
			},
			wantErr:   true,
			errSubstr: "missing or invalid 'addr' field",
		},
		{
			name: "invalid addr format",
			input: map[string]any{
				"state": map[string]any{"addr": "not-an-addr"},
				"rtts":  []any{float64(1)},
			},
			wantErr:   true,
			errSubstr: "parse 'addr' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, rtts, err := parseClusterMemberRTT(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAddr, addr.String())
			assert.Equal(t, tt.wantRTTs, rtts)

			// Sanity: the parsed addr is a valid AddrPort.
			_, perr := netip.ParseAddrPort(addr.String())
			assert.NoError(t, perr)
		})
	}
}
