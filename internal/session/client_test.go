package session

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestGetJSONRetriesOn429 verifies that the session client transparently
// retries requests that come back with HTTP 429, honoring the Retry-After
// header, and ultimately returns the successful response body.
func TestGetJSONRetriesOn429(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0") // immediate retry, no test slowdown
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c, err := New(1000 /*rps*/, 1000 /*burst*/, 5 /*maxRetries429*/)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body, err := c.GetJSON(server.URL)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("expected 3 server hits (2x429 + 1x200), got %d", got)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("unexpected body: %q", string(body))
	}
}

// TestGetJSONGivesUpAfterMaxRetries ensures 429s eventually surface to the
// caller as an error once the retry budget is exhausted.
func TestGetJSONGivesUpAfterMaxRetries(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	const maxRetries = 2
	c, err := New(1000, 1000, maxRetries)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.GetJSON(server.URL); err == nil {
		t.Fatalf("expected error after exhausting retries")
	}

	// First attempt + maxRetries retries = maxRetries+1 hits.
	if got := atomic.LoadInt32(&hits); got != maxRetries+1 {
		t.Errorf("expected %d hits, got %d", maxRetries+1, got)
	}
}

// TestRateLimiterCapsRPS verifies the global limiter actually paces requests:
// firing N back-to-back calls at 5 rps should take at least (N-burst)/rps
// seconds, even if each call would otherwise return instantly.
func TestRateLimiterCapsRPS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	c, err := New(5 /*rps*/, 1 /*burst*/, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 6
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := c.GetJSON(server.URL); err != nil {
			t.Fatalf("GetJSON #%d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// With rps=5, burst=1: first call is free, then 5 calls each waiting ~200ms.
	// Floor is ~ (n-1)/5 seconds. Allow generous slack for slow CI but reject
	// the no-throttling case (which would be near-zero ms).
	min := time.Duration(float64(n-1)/5*float64(time.Second)) - 100*time.Millisecond
	if elapsed < min {
		t.Errorf("expected at least %s with rate limiter, got %s", min, elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"3", 3 * time.Second},
		{"   7   ", 7 * time.Second},
		{"-5", 0},
		{"not a number", 0},
		// HTTP-date in the past → 0
		{time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
	}
	for _, tc := range cases {
		got := parseRetryAfter(tc.in)
		if got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	// HTTP-date in the future → positive
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 {
		t.Errorf("parseRetryAfter(future) = %v, want > 0", got)
	}
}

