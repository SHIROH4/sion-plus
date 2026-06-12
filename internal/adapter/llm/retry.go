package llm

import (
	"math"
	"time"
)

// backoff returns exponential backoff duration: 1s, 2s, 4s, 8s...
// Capped at 30 seconds. attempt is 1-indexed.
func backoff(attempt int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
