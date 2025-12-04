package utils

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mutex           sync.RWMutex
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
}

var (
	globalCircuitBreaker *CircuitBreaker
	circuitBreakerOnce   sync.Once

	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// GetCircuitBreaker returns the singleton circuit breaker
func GetCircuitBreaker() *CircuitBreaker {
	circuitBreakerOnce.Do(func() {
		globalCircuitBreaker = &CircuitBreaker{
			state:            CircuitClosed,
			failureThreshold: 5,
			successThreshold: 2,
			timeout:          30 * time.Second,
		}
	})
	return globalCircuitBreaker
}

// Allow checks if a request is allowed through
func (cb *CircuitBreaker) Allow() error {
	cb.mutex.RLock()
	state := cb.state
	lastFailure := cb.lastFailureTime
	cb.mutex.RUnlock()

	switch state {
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(lastFailure) > cb.timeout {
			cb.mutex.Lock()
			cb.state = CircuitHalfOpen
			cb.successes = 0
			cb.mutex.Unlock()
			return nil
		}
		return ErrCircuitOpen
	case CircuitHalfOpen, CircuitClosed:
		return nil
	}
	return nil
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0

	if cb.state == CircuitHalfOpen {
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = CircuitClosed
		}
	}
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		return
	}

	if cb.failures >= cb.failureThreshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
	}
}

// CircuitBreakerStats holds circuit breaker statistics
type CircuitBreakerStats struct {
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastFailureTime time.Time `json:"last_failure_time,omitempty"`
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.lastFailureTime = time.Time{}
}
