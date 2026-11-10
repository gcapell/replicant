package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

// rateLimited serves 429s (with retryAfter as Retry-After, if not "") for
// the first n requests, then 200 "ok". It records each request's body.
func rateLimited(t *testing.T, n int, retryAfter string) *[]string {
	t.Helper()
	return failing(t, n, http.StatusTooManyRequests, retryAfter)
}

// failing serves status (with retryAfter as Retry-After, if not "") for the
// first n requests, then 200 "ok". It records each request's body.
func failing(t *testing.T, n, status int, retryAfter string) *[]string {
	t.Helper()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(b))
		if len(bodies) <= n {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			io.WriteString(w, `{"status":"`+http.StatusText(status)+`"}`)
			return
		}
		io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	saved := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = saved })
	return &bodies
}

func fakeWait(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	saved := wait
	wait = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	t.Cleanup(func() { wait = saved })
	return &waits
}

func TestRetryOn429(t *testing.T) {
	bodies := rateLimited(t, 2, "3")
	waits := fakeWait(t)
	data, err := post(t.Context(), map[string]string{"command": "travel"}, "/devices/%s", "ABC")
	if err != nil || string(data) != "ok" {
		t.Fatalf("got %q, %v", data, err)
	}
	if want := []time.Duration{3 * time.Second, 3 * time.Second}; !slices.Equal(*waits, want) {
		t.Errorf("waits %v, want %v", *waits, want)
	}
	// The body is sent again each time.
	want := `POST /devices/ABC {"command":"travel"}`
	if len(*bodies) != 3 || (*bodies)[0] != want || (*bodies)[2] != want {
		t.Errorf("requests %q, want 3 of %q", *bodies, want)
	}
}

func TestRetryGivesUp(t *testing.T) {
	bodies := rateLimited(t, 100, "")
	waits := fakeWait(t)
	_, err := get(t.Context(), "/devices")
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want a 429 HTTPError", err)
	}
	if len(*bodies) != maxAttempts || len(*waits) != maxAttempts-1 || (*waits)[0] != defaultRetryAfter {
		t.Errorf("%d requests, waits %v; want %d requests, %d waits of %s", len(*bodies), *waits, maxAttempts, maxAttempts-1, defaultRetryAfter)
	}
}

func TestRetryStopsWithContext(t *testing.T) {
	rateLimited(t, 100, "60")
	// The request gets its 429; the 60s wait is cut short.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := get(ctx, "/devices"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
}

func TestRetryOn503(t *testing.T) {
	bodies := failing(t, 6, http.StatusServiceUnavailable, "1")
	waits := fakeWait(t)
	data, err := post(t.Context(), map[string]string{"command": "travel"}, "/devices/%s", "ABC")
	if err != nil || string(data) != "ok" {
		t.Fatalf("got %q, %v", data, err)
	}
	// Retry-After is ignored; past maxAttempts it carries on.
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 4 * time.Minute, 4 * time.Minute}
	if !slices.Equal(*waits, want) {
		t.Errorf("waits %v, want %v", *waits, want)
	}
	if len(*bodies) != 7 {
		t.Errorf("%d requests, want 7", len(*bodies))
	}
}

func TestRetryOn503ForEver(t *testing.T) {
	failing(t, 1000, http.StatusServiceUnavailable, "")
	waits := fakeWait(t)
	if _, err := get(t.Context(), "/devices"); err != nil {
		t.Fatal(err)
	}
	if len(*waits) != 1000 || (*waits)[999] != 4*time.Minute {
		t.Errorf("%d waits, last %s; want 1000, last 4m", len(*waits), (*waits)[len(*waits)-1])
	}
}

func TestRetryOn503StopsWithContext(t *testing.T) {
	failing(t, 100, http.StatusServiceUnavailable, "")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := get(ctx, "/devices"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		h    string
		want time.Duration
	}{
		{"7", 7 * time.Second},
		{" 0 ", 0},
		{"", defaultRetryAfter},
		{"soon", defaultRetryAfter},
		{"-3", defaultRetryAfter},
		{"Tue, 10 Nov 2026 12:00:30 GMT", 30 * time.Second},
		{"Tue, 10 Nov 2026 11:00:00 GMT", 0},
	} {
		if got, _ := retryAfter(c.h, now); got != c.want {
			t.Errorf("retryAfter(%q) = %s, want %s", c.h, got, c.want)
		}
	}
}
