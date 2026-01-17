package external

import (
	"fmt"
	"sync"
	"time"
)

type RateLimiter struct {
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefillAt time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates limiter for N requests per minute
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	maxTokens := float64(requestsPerMinute)
	refillRate := maxTokens / 60.0 // tokens per second

	return &RateLimiter{
		tokens:       maxTokens, // Start with full bucket
		maxTokens:    maxTokens,
		refillRate:   refillRate,
		lastRefillAt: time.Now(),
	}
}

// WaitForToken blocks until token available, returns wait duration
func (rl *RateLimiter) WaitForToken(maxWait time.Duration) (time.Duration, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefillAt).Seconds()
	tokensToAdd := elapsed * rl.refillRate
	rl.tokens = min(rl.tokens+tokensToAdd, rl.maxTokens)
	rl.lastRefillAt = now

	// If token available, use it immediately
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return 0, nil
	}

	// Calculate wait time for next token
	tokenDeficit := 1.0 - rl.tokens
	waitSeconds := tokenDeficit / rl.refillRate
	waitDuration := time.Duration(waitSeconds * float64(time.Second))

	// Too long to wait?
	if waitDuration > maxWait {
		return 0, fmt.Errorf("rate limit requires %.1fs wait, max allowed: %.1fs",
			waitDuration.Seconds(), maxWait.Seconds())
	}

	// Wait and recurse
	time.Sleep(waitDuration + 100*time.Millisecond) // Small buffer
	additionalWait, err := rl.WaitForToken(maxWait - waitDuration)
	return waitDuration + 100*time.Millisecond + additionalWait, err
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
