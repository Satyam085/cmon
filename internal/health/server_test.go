package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpointStartingState(t *testing.T) {
	mux := http.NewServeMux()
	registerStatusEndpoints(mux, NewMonitor())

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("starting state should return 200, got %d", resp.StatusCode)
	}

	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Status != "starting" {
		t.Errorf("Status: got %q, want starting", s.Status)
	}
	if s.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors: got %d, want 0", s.ConsecutiveErrors)
	}
	if s.LastFetchSuccessAt != "" {
		t.Errorf("LastFetchSuccessAt should be empty before any success, got %q", s.LastFetchSuccessAt)
	}
}

func TestHealthEndpointAfterSuccess(t *testing.T) {
	monitor := NewMonitor()
	monitor.UpdateFetchStatus("success")

	mux := http.NewServeMux()
	registerStatusEndpoints(mux, monitor)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthy state should return 200, got %d", resp.StatusCode)
	}

	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Status != "healthy" {
		t.Errorf("Status: got %q, want healthy", s.Status)
	}
	if s.LastFetchSuccessAt == "" {
		t.Error("LastFetchSuccessAt should be populated after a success")
	}
	if s.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors after success: got %d, want 0", s.ConsecutiveErrors)
	}
}

func TestHealthEndpointConsecutiveErrors(t *testing.T) {
	monitor := NewMonitor()
	// First a success — pins lastFetchSuccessAt and clears the error counter.
	monitor.UpdateFetchStatus("success")
	successAt := monitor.GetStatus().LastFetchSuccessAt

	// Then three failures — counter should advance, success timestamp must
	// not move because the success is the anchor used by alerting probes.
	monitor.UpdateFetchStatus("error: a")
	monitor.UpdateFetchStatus("error: b")
	monitor.UpdateFetchStatus("error: c")

	mux := http.NewServeMux()
	registerStatusEndpoints(mux, monitor)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unhealthy should return 503, got %d", resp.StatusCode)
	}

	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Status != "unhealthy" {
		t.Errorf("Status: got %q, want unhealthy", s.Status)
	}
	if s.ConsecutiveErrors != 3 {
		t.Errorf("ConsecutiveErrors: got %d, want 3", s.ConsecutiveErrors)
	}
	if s.LastFetchSuccessAt != successAt {
		t.Errorf("LastFetchSuccessAt should not move on failure: got %q, want %q", s.LastFetchSuccessAt, successAt)
	}
	if !strings.HasPrefix(s.LastFetchStatus, "error:") {
		t.Errorf("LastFetchStatus: got %q, want error: prefix", s.LastFetchStatus)
	}
}

func TestSuccessResetsConsecutiveErrors(t *testing.T) {
	monitor := NewMonitor()
	monitor.UpdateFetchStatus("error: x")
	monitor.UpdateFetchStatus("error: y")
	if got := monitor.GetStatus().ConsecutiveErrors; got != 2 {
		t.Fatalf("setup: ConsecutiveErrors=%d, want 2", got)
	}

	monitor.UpdateFetchStatus("success")
	got := monitor.GetStatus()
	if got.ConsecutiveErrors != 0 {
		t.Errorf("success must reset ConsecutiveErrors to 0, got %d", got.ConsecutiveErrors)
	}
	if got.Status != "healthy" {
		t.Errorf("Status after recovery: got %q, want healthy", got.Status)
	}
}
