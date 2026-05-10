package session

import (
	"encoding/json"
	"fmt"
	"io"
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

// TestSolveCaptcha covers the arithmetic captcha used on the DGVCL portal
// login page. Each row is one input → expected outcome. Sad-path rows
// expect an error and use empty `want`.
func TestSolveCaptcha(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"plain addition", "5 + 3", "8", false},
		{"plain subtraction", "12 - 4", "8", false},
		{"unicode multiplication", "3 × 7", "21", false},
		{"ascii x multiplication", "3 x 7", "21", false},
		{"capital X multiplication", "3 X 7", "21", false},
		{"asterisk multiplication", "4 * 6", "24", false},
		{"no spaces around operator", "10+5", "15", false},
		{"surrounding whitespace", "   8 - 2   ", "6", false},
		{"two-digit operands", "100 + 250", "350", false},
		{"zero subtrahend", "9 - 0", "9", false},
		{"negative result is allowed", "3 - 10", "-7", false},
		{"empty string", "", "", true},
		{"only operand", "42", "", true},
		{"only operator", "+", "", true},
		{"two operands no operator", "5 5", "", true},
		{"unsupported operator slash", "10 / 2", "", true},
		{"non-numeric operand", "five + 3", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := solveCaptcha(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("solveCaptcha(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("solveCaptcha(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("solveCaptcha(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// loginFixture stands up a fake DGVCL portal that serves a login page with a
// captcha + CSRF token, accepts a JSON POST to /api/login, and returns either
// a token or an error depending on whether captcha + credentials are right.
type loginFixture struct {
	server      *httptest.Server
	loginHits   int32
	apiHits     int32
	captchaText string
	csrfToken   string
	wantUser    string
	wantPass    string
	apiResponse func(w http.ResponseWriter, body []byte) // override per test
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	f := &loginFixture{
		captchaText: "5 + 7",
		csrfToken:   "fixture-csrf-token",
		wantUser:    "user1",
		wantPass:    "pw1",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.loginHits, 1)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!doctype html><html><head>
<meta name="csrf-token" content="%s">
</head><body>
<ul><li class="captchaList"><span>%s</span></li></ul>
<input id="email_or_username" name="email_or_username">
</body></html>`, f.csrfToken, f.captchaText)
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.apiHits, 1)
		body, _ := io.ReadAll(r.Body)
		if f.apiResponse != nil {
			f.apiResponse(w, body)
			return
		}
		var payload struct {
			EmailOrUsername string `json:"email_or_username"`
			Password        string `json:"password"`
			Captcha         string `json:"captcha"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		// CSRF header round-trips back so tests can assert it.
		w.Header().Set("X-Saw-CSRF", r.Header.Get("X-CSRF-TOKEN"))
		if payload.EmailOrUsername != f.wantUser || payload.Password != f.wantPass {
			http.Error(w, "bad creds", http.StatusUnauthorized)
			return
		}
		if payload.Captcha != "12" { // 5 + 7
			http.Error(w, "bad captcha", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"fixture-bearer"}`)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// TestLoginHappyPathSendsCsrfAndStoresToken exercises the full login flow:
// captcha is solved, the JSON POST carries the CSRF header, and the returned
// bearer token is captured for subsequent authenticated calls.
func TestLoginHappyPathSendsCsrfAndStoresToken(t *testing.T) {
	f := newLoginFixture(t)

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(f.server.URL+"/login", f.wantUser, f.wantPass); err != nil {
		t.Fatalf("Login: %v", err)
	}

	if got := atomic.LoadInt32(&f.loginHits); got != 1 {
		t.Errorf("login page hits: got %d, want 1", got)
	}
	if got := atomic.LoadInt32(&f.apiHits); got != 1 {
		t.Errorf("api/login hits: got %d, want 1", got)
	}

	c.mu.RLock()
	token := c.bearerToken
	c.mu.RUnlock()
	if token != "fixture-bearer" {
		t.Errorf("bearerToken: got %q, want fixture-bearer", token)
	}
}

// TestLoginFailsOnUnsolvableCaptcha verifies that a captcha the solver
// cannot parse aborts the flow before any credential POST.
func TestLoginFailsOnUnsolvableCaptcha(t *testing.T) {
	f := newLoginFixture(t)
	f.captchaText = "garbled" // solver returns an error

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(f.server.URL+"/login", f.wantUser, f.wantPass); err == nil {
		t.Fatal("expected error from Login when captcha is unsolvable")
	}

	if got := atomic.LoadInt32(&f.apiHits); got != 0 {
		t.Errorf("api/login should not be called when captcha unsolvable; got %d hits", got)
	}
}

// TestLoginFailsOnMissingCaptcha verifies that an empty captcha element on
// the login page aborts the flow with a clear error.
func TestLoginFailsOnMissingCaptcha(t *testing.T) {
	f := newLoginFixture(t)
	f.captchaText = "" // selector returns empty

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(f.server.URL+"/login", f.wantUser, f.wantPass); err == nil {
		t.Fatal("expected error when captcha element is empty")
	}
	if got := atomic.LoadInt32(&f.apiHits); got != 0 {
		t.Errorf("api/login should not be called; got %d hits", got)
	}
}

// TestLoginFailsOnBadCredentials surfaces the upstream HTTP error when the
// portal rejects the credentials.
func TestLoginFailsOnBadCredentials(t *testing.T) {
	f := newLoginFixture(t)

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(f.server.URL+"/login", "wrong", "wrong"); err == nil {
		t.Fatal("expected error for bad credentials")
	}

	c.mu.RLock()
	token := c.bearerToken
	c.mu.RUnlock()
	if token != "" {
		t.Errorf("bearerToken should be empty after failed login; got %q", token)
	}
}

// TestLoginFailsWhenApiResponseMissingToken handles the case where the API
// returns 200 but the JSON body has no token field. This is a real-world
// failure mode the production code must not silently accept.
func TestLoginFailsWhenApiResponseMissingToken(t *testing.T) {
	f := newLoginFixture(t)
	f.apiResponse = func(w http.ResponseWriter, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"message":"ok but token-less"}`)
	}

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Login(f.server.URL+"/login", f.wantUser, f.wantPass); err == nil {
		t.Fatal("expected error when API response lacks token")
	}
}

// TestIsSessionExpiredDetectsLoginForm verifies the dashboard probe
// correctly classifies a response containing the login form as expired.
func TestIsSessionExpiredDetectsLoginForm(t *testing.T) {
	withLoginForm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><input id="email_or_username"></body></html>`)
	}))
	defer withLoginForm.Close()

	withoutLoginForm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1>Dashboard</h1></body></html>`)
	}))
	defer withoutLoginForm.Close()

	c, err := New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !c.IsSessionExpired(withLoginForm.URL) {
		t.Error("expected expired=true when login form is present")
	}
	if c.IsSessionExpired(withoutLoginForm.URL) {
		t.Error("expected expired=false when login form is absent")
	}
}

// Sanity: solveCaptcha must compute exactly what the API expects for the
// fixture text. Catches accidental drift in the parser.
func TestLoginCaptchaSolverMatchesFixtureExpectation(t *testing.T) {
	got, err := solveCaptcha("5 + 7")
	if err != nil {
		t.Fatalf("solveCaptcha: %v", err)
	}
	if got != "12" {
		t.Errorf("solveCaptcha(5+7) = %q, want 12", got)
	}
}


