package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
)

func TestFlareSolverrCancellationKeepsSlotUntilSolverFinishes(t *testing.T) {
	for _, expire := range []bool{false, true} {
		name := "cancel"
		if expire {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			previous := config.AppConfig
			config.AppConfig.FlareSolverrMaxConcurrent = 1
			t.Cleanup(func() { config.AppConfig = previous })
			var requests atomic.Int32
			started := make(chan struct{})
			disconnected := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if requests.Add(1) == 1 {
					close(started)
					select {
					case <-r.Context().Done():
						close(disconnected)
						<-release
					case <-release:
					}
				}
				_, _ = io.WriteString(w, `{"status":"ok","solution":{"status":200,"response":"<html>solved</html>",`+
					`"url":"https://baak.example/","cookies":[{"name":"session","value":"late","path":"/"}]}}`)
			}))
			ctx, cancel := context.WithCancel(context.Background())
			if expire {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			result := make(chan error, 1)
			defer func() {
				cancel()
				unblock()
				server.Close()
				waitForFlareSolverrIdle(t)
			}()
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			target, _ := url.Parse("https://baak.example/")
			jar.SetCookies(target, []*http.Cookie{{Name: "session", Value: "original", Path: "/"}})
			fs := &FlareSolverr{url: server.URL, client: server.Client()}
			go func() {
				_, err := fs.FetchContext(ctx, target.String(), jar)
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("first solver job did not start")
			}
			if !expire {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, ctx.Err()) {
					t.Fatalf("canceled request error = %v, want %v", err, ctx.Err())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("canceled caller waited for the browser job")
			}

			probeCtx, cancelProbe := context.WithTimeout(context.Background(), time.Second)
			defer cancelProbe()
			other := &FlareSolverr{url: server.URL, client: server.Client()}
			if _, err := other.FetchContext(probeCtx, target.String(), nil); !errors.Is(err, ErrFlareSolverrBusy) {
				t.Errorf("another scraper started before the canceled job finished: %v", err)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("solver received %d requests while the first job was still running, want 1", got)
			}
			select {
			case <-disconnected:
				t.Error("caller cancellation disconnected the independent solver request")
			default:
			}

			unblock()
			waitForFlareSolverrIdle(t)
			if _, err := other.FetchContext(probeCtx, target.String(), nil); err != nil {
				t.Fatalf("finished solver job did not release capacity: %v", err)
			}
			cookies := jar.Cookies(target)
			if len(cookies) != 1 || cookies[0].Value != "original" {
				t.Fatalf("late result changed the canceled caller's cookies: %v", cookies)
			}
		})
	}
}

func TestFlareSolverrWorkerReleasesCapacityAfterFailure(t *testing.T) {
	for _, failure := range []string{"transport", "timeout", "malformed JSON", "panic"} {
		t.Run(failure, func(t *testing.T) {
			previous := config.AppConfig
			config.AppConfig.FlareSolverrMaxConcurrent = 1
			t.Cleanup(func() { config.AppConfig = previous })
			var calls atomic.Int32
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if _, ok := r.Context().Deadline(); !ok {
					t.Error("solver job has no independent deadline")
				}
				if calls.Add(1) == 1 {
					switch failure {
					case "transport":
						return nil, errors.New("fixture transport failure")
					case "timeout":
						<-r.Context().Done()
						return nil, r.Context().Err()
					case "malformed JSON":
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{"))}, nil
					case "panic":
						panic("private transport payload")
					}
				}
				return &http.Response{
					StatusCode: 200,
					Body: io.NopCloser(strings.NewReader(
						`{"status":"ok","solution":{"status":200,"response":"<html>recovered</html>"}}`,
					)),
				}, nil
			})}
			if failure == "timeout" {
				client.Timeout = 20 * time.Millisecond
			}
			fs := &FlareSolverr{url: "http://solver.example", client: client}
			_, err := fs.FetchContext(context.Background(), "https://baak.example", nil)
			if err == nil {
				t.Fatal("solver failure was accepted")
			}
			if strings.Contains(err.Error(), "private transport payload") {
				t.Fatal("worker panic exposed its payload")
			}
			if failure == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timeout error = %v, want deadline exceeded", err)
			}
			html, err := fs.FetchContext(context.Background(), "https://baak.example", nil)
			if err != nil || html != "<html>recovered</html>" {
				t.Fatalf("failed solver job left capacity reserved: %q, %v", html, err)
			}
		})
	}
}

func TestFlareSolverrAlreadyCanceledCallerDoesNotStartWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fs := &FlareSolverr{
		url: "http://solver.example",
		client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			t.Error("already canceled caller started a solver job")
			return nil, errors.New("unexpected call")
		})},
	}
	_, err := fs.FetchContext(ctx, "https://baak.example", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller error = %v", err)
	}
}

func waitForFlareSolverrIdle(t *testing.T) {
	t.Helper()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		flareConcurrency.Lock()
		active := flareConcurrency.active
		flareConcurrency.Unlock()
		if active == 0 {
			return
		}
		select {
		case <-timeout.C:
			t.Fatal("solver job did not release its slot")
		case <-ticker.C:
		}
	}
}
