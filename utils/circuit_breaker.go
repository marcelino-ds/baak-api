package utils

import (
	"context"
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
	halfOpenProbe   bool
	generation      uint64
	lastFailureTime time.Time

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
}

var (
	globalCircuitBreaker *CircuitBreaker
	circuitBreakerOnce   sync.Once
	flareCircuitBreaker  = NewCircuitBreaker()

	ErrCircuitOpen = errors.New("circuit breaker is open")
)

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: 5,
		successThreshold: 2,
		timeout:          30 * time.Second,
	}
}

// GetCircuitBreaker returns the singleton circuit breaker
func GetCircuitBreaker() *CircuitBreaker {
	circuitBreakerOnce.Do(func() {
		globalCircuitBreaker = NewCircuitBreaker()
	})
	return globalCircuitBreaker
}

// GetFlareCircuitBreaker shares dependency failures across request-scoped scrapers.
func GetFlareCircuitBreaker() *CircuitBreaker {
	return flareCircuitBreaker
}

// Allow checks if a request is allowed through
func (cb *CircuitBreaker) Allow() error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	return cb.allowLocked()
}

func (cb *CircuitBreaker) allowLocked() error {
	switch cb.state {
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) <= cb.timeout {
			return ErrCircuitOpen
		}
		cb.state = CircuitHalfOpen
		cb.generation++
		cb.successes = 0
		cb.halfOpenProbe = true
		return nil
	case CircuitHalfOpen:
		// Recovery probes are serialized to avoid flooding a recovering upstream.
		if cb.halfOpenProbe {
			return ErrCircuitOpen
		}
		cb.halfOpenProbe = true
		return nil
	case CircuitClosed:
		return nil
	default:
		return ErrCircuitOpen
	}
}

// Execute tracks an operation in its circuit generation. Late completions from
// before an opening or reset cannot close the circuit or release a newer probe.
func (cb *CircuitBreaker) Execute(operation func() error) (err error) {
	cb.mutex.Lock()
	if err = cb.allowLocked(); err != nil {
		cb.mutex.Unlock()
		return err
	}
	generation := cb.generation
	cb.mutex.Unlock()
	err = errors.New("circuit operation did not complete")
	defer func() {
		cb.mutex.Lock()
		defer cb.mutex.Unlock()
		if generation != cb.generation {
			return
		}
		if errors.Is(err, context.Canceled) {
			cb.halfOpenProbe = false
			return
		}
		var status upstreamStatusError
		if err == nil || (errors.As(err, &status) && status.code >= 400 && status.code < 500 && status.code != 403 && status.code != 408 && status.code != 429) {
			cb.recordSuccessLocked()
		} else {
			cb.recordFailureLocked()
		}
	}()
	return operation()
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.recordSuccessLocked()
}

func (cb *CircuitBreaker) recordSuccessLocked() {
	if cb.state == CircuitOpen {
		return
	}
	if cb.state == CircuitHalfOpen {
		cb.halfOpenProbe = false
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = CircuitClosed
			cb.generation++
			cb.successes = 0
		}
	}
	cb.failures = 0
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.recordFailureLocked()
}

func (cb *CircuitBreaker) recordFailureLocked() {
	if cb.state == CircuitOpen {
		return
	}
	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenProbe = false
		cb.state = CircuitOpen
		cb.generation++
		return
	}

	if cb.failures >= cb.failureThreshold {
		cb.state = CircuitOpen
		cb.generation++
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

	var lastFailure *time.Time
	if !cb.lastFailureTime.IsZero() {
		value := cb.lastFailureTime
		lastFailure = &value
	}
	return CircuitBreakerStats{
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: lastFailure,
		// Readiness must admit traffic after cooldown so a recovery probe can run.
		AcceptingRequests: cb.state == CircuitClosed ||
			(cb.state == CircuitOpen && time.Since(cb.lastFailureTime) > cb.timeout) ||
			(cb.state == CircuitHalfOpen && !cb.halfOpenProbe),
	}
}

// CircuitBreakerStats holds circuit breaker statistics
type CircuitBreakerStats struct {
	State             string     `json:"state"`
	Failures          int        `json:"failures"`
	Successes         int        `json:"successes"`
	LastFailureTime   *time.Time `json:"last_failure_time,omitempty"`
	AcceptingRequests bool       `json:"accepting_requests"`
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = CircuitClosed
	cb.generation++
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenProbe = false
	cb.lastFailureTime = time.Time{}
}
