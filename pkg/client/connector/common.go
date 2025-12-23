package connector

import (
	"encoding/json"
	"fmt"
)

// defaultServiceConfig defines the default gRPC service configuration including retry policy for transient failures.
var defaultServiceConfig = mustMarshalJSON(map[string]any{
	"methodConfig": []map[string]any{
		{
			"name": []map[string]string{{"service": ""}},
			"retryPolicy": map[string]any{
				"maxAttempts":          5, // 5 is the maximum allowed by gRPC
				"initialBackoff":       "0.5s",
				"maxBackoff":           "5s",
				"backoffMultiplier":    2,
				"retryableStatusCodes": []string{"UNAVAILABLE"},
			},
		},
	},
})

func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal service config: %v", err))
	}
	return string(b)
}
