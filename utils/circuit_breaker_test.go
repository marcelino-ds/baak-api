package utils

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLocalSolverOverloadDoesNotChangeDependencyFailures(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.RecordFailure()
	for range 10 {
		err := cb.Execute(func() error { return fmt.Errorf("%w: %w", ErrFlareSolverr, ErrFlareSolverrBusy) })
		if !errors.Is(err, ErrFlareSolverrBusy) {
			t.Fatalf("local capacity error opened circuit: %v", err)
		}
	}
	if stats := cb.Stats(); stats.Failures != 1 || cb.State() != CircuitClosed {
		t.Fatalf("overload changed dependency failures: %+v", stats)
	}
}

func TestOverloadedRecoveryProbeCanBeRetried(t *testing.T) {
	cb := &CircuitBreaker{
		state: CircuitOpen, lastFailureTime: time.Now().Add(-time.Minute),
		timeout: time.Second, successThreshold: 1,
	}
	_ = cb.Execute(func() error { return ErrFlareSolverrBusy })
	if err := cb.Execute(func() error { return nil }); err != nil {
		t.Fatalf("local capacity error prevented another recovery probe: %v", err)
	}
	if cb.State() != CircuitClosed {
		t.Fatal("successful probe did not recover the circuit")
	}
}

func TestCircuitBreakerAllowsOneHalfOpenProbe(t *testing.T) {
	cb := &CircuitBreaker{
		state:            CircuitOpen,
		failureThreshold: 1,
		successThreshold: 2,
		timeout:          time.Second,
		lastFailureTime:  time.Now().Add(-2 * time.Second),
	}

	var allowed int
	var mu sync.Mutex
	var calls sync.WaitGroup
	for range 20 {
		calls.Add(1)
		go func() {
			defer calls.Done()
			if cb.Allow() == nil {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	calls.Wait()

	if allowed != 1 {
		t.Fatalf("half-open allowed %d probes, want 1", allowed)
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("state = %s, want half-open", cb.State())
	}
}

func TestCircuitBreakerClosesAfterRecoveryProbes(t *testing.T) {
	cb := &CircuitBreaker{
		state:            CircuitOpen,
		successThreshold: 2,
		timeout:          time.Second,
		lastFailureTime:  time.Now().Add(-2 * time.Second),
	}

	if err := cb.Allow(); err != nil {
		t.Fatalf("first recovery probe rejected: %v", err)
	}
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("state after first success = %s, want half-open", cb.State())
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("second recovery probe rejected: %v", err)
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("state after recovery = %s, want closed", cb.State())
	}
}

func TestCircuitReadinessRecoversAfterCooldownWithoutReservingProbe(t *testing.T) {
	cb := &CircuitBreaker{
		state:            CircuitOpen,
		successThreshold: 2,
		timeout:          time.Hour,
		lastFailureTime:  time.Now(),
	}
	if cb.Stats().AcceptingRequests {
		t.Fatal("circuit accepted traffic during cooldown")
	}
	cb.lastFailureTime = time.Now().Add(-2 * time.Hour)
	for range 3 {
		if !cb.Stats().AcceptingRequests || cb.State() != CircuitOpen {
			t.Fatal("readiness must allow recovery traffic without consuming the probe")
		}
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("readiness prevented recovery: %v", err)
	}
	if cb.Stats().AcceptingRequests {
		t.Fatal("another probe was permitted while recovery was in progress")
	}
	cb.RecordSuccess()
	if !cb.Stats().AcceptingRequests {
		t.Fatal("readiness blocked the second recovery probe")
	}
	if err := cb.Execute(func() error { return nil }); err != nil || cb.State() != CircuitClosed {
		t.Fatalf("circuit did not recover: state=%s, err=%v", cb.State(), err)
	}
}

func TestCircuitBreakerDoesNotCountCanceledOperationAsFailure(t *testing.T) {
	cb := &CircuitBreaker{state: CircuitClosed, failureThreshold: 1, successThreshold: 1, timeout: time.Second}
	if err := cb.Execute(func() error { return context.Canceled }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context canceled", err)
	}
	stats := cb.Stats()
	if stats.State != "closed" || stats.Failures != 0 {
		t.Fatalf("canceled operation changed breaker: %+v", stats)
	}
}

func TestCircuitIgnoresCompletionFromBeforeReset(t *testing.T) {
	cb := &CircuitBreaker{failureThreshold: 1, successThreshold: 1, timeout: time.Hour}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = cb.Execute(func() error { close(started); <-release; return errors.New("old failure") })
	}()
	<-started
	cb.Reset()
	close(release)
	<-done
	if cb.State() != CircuitClosed {
		t.Fatal("old request reopened reset circuit")
	}
}

func TestCanceledRecoveryProbeCanBeRetried(t *testing.T) {
	cb := &CircuitBreaker{state: CircuitOpen, lastFailureTime: time.Now().Add(-time.Hour), timeout: time.Second, successThreshold: 1}
	_ = cb.Execute(func() error { return context.Canceled })
	if err := cb.Execute(func() error { return nil }); err != nil {
		t.Fatalf("recovery probe stayed locked: %v", err)
	}
	if cb.State() != CircuitClosed {
		t.Fatal("circuit did not recover")
	}
}

func TestCircuitCountsTimeoutsAndReleasesPanickedProbe(t *testing.T) {
	cb := &CircuitBreaker{failureThreshold: 1, timeout: time.Hour, successThreshold: 1}
	_ = cb.Execute(func() error { return context.DeadlineExceeded })
	if cb.State() != CircuitOpen {
		t.Fatal("upstream timeout was not counted")
	}
	cb.Reset()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		_ = cb.Execute(func() error { panic("fixture") })
	}()
	if cb.State() != CircuitOpen {
		t.Fatal("panic did not release the operation")
	}
}
