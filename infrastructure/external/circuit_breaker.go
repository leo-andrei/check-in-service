package external

import (
	"fmt"
	"sync"
	"time"
)

type CircuitState string

const (
	StateClosed CircuitState = "CLOSED" // Normal operation
	StateOpen   CircuitState = "OPEN"   // Failing, reject requests
	StateHalf   CircuitState = "HALF"   // Testing if service recovered
)

// CircuitBreaker prevents cascading failures to external services
type CircuitBreaker struct {
	state            CircuitState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	mu               sync.RWMutex
}

func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

// RecordSuccess records a successful call
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0

	if cb.state == StateHalf {
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = StateClosed
			cb.successCount = 0
			fmt.Printf("Circuit breaker CLOSED - service recovered\n")
		}
	}
}

// RecordFailure records a failed call
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()
	cb.successCount = 0

	if cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
		fmt.Printf("Circuit breaker OPEN - too many failures (%d)\n", cb.failureCount)
	}
}

// CanExecute checks if a request can be attempted
func (cb *CircuitBreaker) CanExecute() (bool, error) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true, nil

	case StateOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.timeout {
			// Try to recover
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalf
			cb.failureCount = 0
			fmt.Printf("Circuit breaker HALF-OPEN - testing recovery\n")
			cb.mu.Unlock()
			cb.mu.RLock()
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker is OPEN - service unavailable")

	case StateHalf:
		// Allow test request
		return true, nil

	default:
		return false, fmt.Errorf("unknown circuit breaker state: %s", cb.state)
	}
}

// GetState returns the current state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
