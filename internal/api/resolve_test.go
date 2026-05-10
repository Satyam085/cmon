package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"cmon/internal/metrics"
	"cmon/internal/session"
)

// withEndpoint swaps the package-level resolveEndpoint for the test and
// restores the original on cleanup. The production URL is hard-coded for
// now; the broader runtime override is tracked in T4.3.
func withEndpoint(t *testing.T, url string) {
	t.Helper()
	prev := resolveEndpoint
	resolveEndpoint = url
	t.Cleanup(func() { resolveEndpoint = prev })
}

func newTestClient(t *testing.T) *session.Client {
	t.Helper()
	sc, err := session.New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return sc
}

// TestResolveComplaintSendsExpectedFormFields verifies the wire shape that
// the DGVCL API expects: POST x-www-form-urlencoded with complaint_id,
// complaint_AsignType=resolved, and the user's remark.
func TestResolveComplaintSendsExpectedFormFields(t *testing.T) {
	type captured struct {
		method      string
		contentType string
		complaintID string
		assignType  string
		remark      string
	}
	var got captured
	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		got.method = r.Method
		got.contentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got.complaintID = r.PostFormValue("complaint_id")
		got.assignType = r.PostFormValue("complaint_AsignType")
		got.remark = r.PostFormValue("remark")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	if err := ResolveComplaint(newTestClient(t), "API-123", "fixed at site", false); err != nil {
		t.Fatalf("ResolveComplaint: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method: got %q, want POST", got.method)
	}
	if !strings.HasPrefix(got.contentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type: got %q, want application/x-www-form-urlencoded prefix", got.contentType)
	}
	if got.complaintID != "API-123" {
		t.Errorf("complaint_id: got %q, want API-123", got.complaintID)
	}
	if got.assignType != "resolved" {
		t.Errorf("complaint_AsignType: got %q, want resolved", got.assignType)
	}
	if got.remark != "fixed at site" {
		t.Errorf("remark: got %q, want %q", got.remark, "fixed at site")
	}
	if h := atomic.LoadInt32(&hits); h != 1 {
		t.Errorf("server hits: got %d, want 1", h)
	}
}

// TestResolveComplaintSurfacesERRORResponse verifies the contract that the
// DGVCL portal signals an application-level failure with an "ERROR:" prefix
// in a 200 response — must turn into a Go error, not a silent OK.
func TestResolveComplaintSurfacesERRORResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ERROR: complaint already resolved"))
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	err := ResolveComplaint(newTestClient(t), "API-1", "note", false)
	if err == nil {
		t.Fatal("expected error for ERROR: response, got nil")
	}
	if !strings.Contains(err.Error(), "complaint already resolved") {
		t.Errorf("error should carry post-prefix text; got %q", err.Error())
	}
}

// TestResolveComplaintSurfacesNon200 verifies HTTP-layer errors (e.g. 500)
// are surfaced rather than silently treated as success.
func TestResolveComplaintSurfacesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	if err := ResolveComplaint(newTestClient(t), "API-1", "note", false); err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

// TestResolveComplaintDebugModeSkipsRequest verifies debugMode=true short-
// circuits before any network call so dry-run usage cannot accidentally
// mutate production state.
func TestResolveComplaintDebugModeSkipsRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	if err := ResolveComplaint(newTestClient(t), "API-1", "note", true); err != nil {
		t.Fatalf("debug-mode ResolveComplaint should not error: %v", err)
	}
	if h := atomic.LoadInt32(&hits); h != 0 {
		t.Errorf("debug mode must not reach upstream; got %d hits", h)
	}
}

// TestResolveComplaintMetricsHappyPath asserts the success counter advances
// once and the failure counter does not.
func TestResolveComplaintMetricsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	calls0 := metrics.ResolveCallsTotal.Value()
	fails0 := metrics.ResolveFailuresTotal.Value()

	if err := ResolveComplaint(newTestClient(t), "API-1", "note", false); err != nil {
		t.Fatalf("ResolveComplaint: %v", err)
	}

	if got := metrics.ResolveCallsTotal.Value(); got != calls0+1 {
		t.Errorf("ResolveCallsTotal: got %d, want %d", got, calls0+1)
	}
	if got := metrics.ResolveFailuresTotal.Value(); got != fails0 {
		t.Errorf("ResolveFailuresTotal must not advance on success; got %d, want %d", got, fails0)
	}
}

// TestResolveComplaintMetricsErrorResponse asserts the failure counter
// advances when the API returns an "ERROR:" response — and that the
// attempt counter still ticks once (we did make the call).
func TestResolveComplaintMetricsErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ERROR: something broke"))
	}))
	defer srv.Close()
	withEndpoint(t, srv.URL)

	calls0 := metrics.ResolveCallsTotal.Value()
	fails0 := metrics.ResolveFailuresTotal.Value()

	_ = ResolveComplaint(newTestClient(t), "API-1", "note", false)

	if got := metrics.ResolveCallsTotal.Value(); got != calls0+1 {
		t.Errorf("ResolveCallsTotal must still advance once on attempt; got %d, want %d", got, calls0+1)
	}
	if got := metrics.ResolveFailuresTotal.Value(); got != fails0+1 {
		t.Errorf("ResolveFailuresTotal: got %d, want %d", got, fails0+1)
	}
}

// TestResolveComplaintMetricsTransportError asserts the failure counter
// advances on a transport-level error (no upstream listener).
func TestResolveComplaintMetricsTransportError(t *testing.T) {
	// Point the endpoint at a port nobody is listening on. Using a closed
	// httptest server gives us a guaranteed-bad URL without any race.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()
	withEndpoint(t, srv.URL)

	calls0 := metrics.ResolveCallsTotal.Value()
	fails0 := metrics.ResolveFailuresTotal.Value()

	if err := ResolveComplaint(newTestClient(t), "API-1", "note", false); err == nil {
		t.Fatal("expected transport error, got nil")
	}

	if got := metrics.ResolveCallsTotal.Value(); got != calls0+1 {
		t.Errorf("ResolveCallsTotal: got %d, want %d", got, calls0+1)
	}
	if got := metrics.ResolveFailuresTotal.Value(); got != fails0+1 {
		t.Errorf("ResolveFailuresTotal: got %d, want %d", got, fails0+1)
	}
}

func TestSetResolveEndpoint(t *testing.T) {
	prev := resolveEndpoint
	t.Cleanup(func() { resolveEndpoint = prev })

	SetResolveEndpoint("https://staging.example/api/x")
	if resolveEndpoint != "https://staging.example/api/x" {
		t.Errorf("SetResolveEndpoint should install the URL; got %q", resolveEndpoint)
	}

	// Empty must be a no-op so a misconfigured deploy can't blank-out
	// the endpoint silently.
	SetResolveEndpoint("")
	if resolveEndpoint != "https://staging.example/api/x" {
		t.Errorf("SetResolveEndpoint(\"\") should not blank the endpoint; got %q", resolveEndpoint)
	}
}

// TestResolveComplaintDebugModeDoesNotMoveMetrics asserts the call counter
// does NOT tick in debug mode — otherwise dry runs would inflate the
// visible API rate.
func TestResolveComplaintDebugModeDoesNotMoveMetrics(t *testing.T) {
	calls0 := metrics.ResolveCallsTotal.Value()
	fails0 := metrics.ResolveFailuresTotal.Value()

	if err := ResolveComplaint(newTestClient(t), "API-1", "note", true); err != nil {
		t.Fatalf("debug-mode ResolveComplaint: %v", err)
	}

	if got := metrics.ResolveCallsTotal.Value(); got != calls0 {
		t.Errorf("ResolveCallsTotal should not move in debug mode; got %d, want %d", got, calls0)
	}
	if got := metrics.ResolveFailuresTotal.Value(); got != fails0 {
		t.Errorf("ResolveFailuresTotal should not move in debug mode; got %d, want %d", got, fails0)
	}
}
