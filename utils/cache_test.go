package utils

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCacheWaiterCancellationLeavesLoadRunning(t *testing.T) {
	cache := &Cache{items: make(map[string]CacheItem)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	loaded := make(chan error, 1)
	go func() {
		_, err := cache.GetOrLoad(ctx, "schedule", time.Minute, func(ctx context.Context) (interface{}, error) {
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return "schedule data", nil
			}
		})
		loaded <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("load did not start")
	}
	waiterCtx, cancelWaiter := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelWaiter()
	_, err := cache.GetOrLoad(waiterCtx, "schedule", time.Minute, func(context.Context) (interface{}, error) {
		t.Error("waiting request started another load")
		return "unexpected", nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting request error = %v, want deadline exceeded", err)
	}
	unblock()
	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("original load: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("original load did not complete")
	}
	if value, ok := cache.Get("schedule"); !ok || value != "schedule data" {
		t.Fatalf("cache after waiter cancellation = %v, %t", value, ok)
	}
}

func TestCacheDoesNotStoreCanceledLoad(t *testing.T) {
	cache := &Cache{items: make(map[string]CacheItem)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := cache.GetOrLoad(ctx, "schedule", time.Minute, func(context.Context) (interface{}, error) {
		cancel()
		return "late response", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("load error = %v, want canceled", err)
	}
	if _, ok := cache.Get("schedule"); ok {
		t.Fatal("canceled load populated cache")
	}
	value, err := cache.GetOrLoad(context.Background(), "schedule", time.Minute,
		func(context.Context) (interface{}, error) { return "fresh response", nil },
	)
	if err != nil || value != "fresh response" {
		t.Fatalf("retry result = %v, %v", value, err)
	}
}

func TestCachePanicDoesNotBlockFutureLoads(t *testing.T) {
	cache := &Cache{items: make(map[string]CacheItem)}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("loader panic was swallowed")
			}
		}()
		_, _ = cache.GetOrLoad(context.Background(), "schedule", time.Minute,
			func(context.Context) (interface{}, error) { panic("fixture panic") },
		)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := cache.GetOrLoad(ctx, "schedule", time.Minute,
		func(context.Context) (interface{}, error) { return "recovered", nil },
	)
	if err != nil || value != "recovered" {
		t.Fatalf("load after panic = %v, %v", value, err)
	}
}

func TestCacheEvictsEntryNearestExpirationAtCapacity(t *testing.T) {
	cache := &Cache{items: make(map[string]CacheItem), maxEntries: 2}
	cache.Set("short", "short", time.Second)
	cache.Set("long", "long", time.Minute)
	cache.Set("new", "new", time.Minute)
	if _, ok := cache.Get("short"); ok {
		t.Fatal("cache retained the entry nearest expiration")
	}
	if _, ok := cache.Get("long"); !ok {
		t.Fatal("cache evicted a valid long-lived entry")
	}
}

func TestCacheWaiterRetriesCanceledLeader(t *testing.T) {
	cache := &Cache{}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	started, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = cache.GetOrLoad(leaderCtx, "key", time.Minute, func(ctx context.Context) (interface{}, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	timer := time.AfterFunc(20*time.Millisecond, cancelLeader)
	defer timer.Stop()
	value, err := cache.GetOrLoad(ctx, "key", time.Minute, func(context.Context) (interface{}, error) { return "replacement", nil })
	<-finished
	if err != nil || value != "replacement" {
		t.Fatalf("waiter inherited leader cancellation: %v, %v", value, err)
	}
}
