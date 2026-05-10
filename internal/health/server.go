// Package health provides health check and monitoring for the CMON application.
//
// This package implements:
//   - HTTP health check endpoint
//   - Application metrics tracking
//   - Uptime monitoring
//   - Status reporting
package health

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"cmon/internal/metrics"
	"cmon/internal/session"
	"cmon/internal/storage"
)

// Status represents the application health status.
//
// This is returned by the /health endpoint for monitoring tools.
//
// Fields:
//   - Status: Overall health status ("healthy", "unhealthy", "starting")
//   - Uptime: How long the application has been running
//   - LastFetchTime: When the last complaint fetch completed (success or fail)
//   - LastFetchStatus: Status of last fetch ("success" or error message)
//   - LastFetchSuccessAt: When the last *successful* fetch completed (empty
//     until the first success). Lets external probes detect a stuck scraper
//     that recently errored even if LastFetchTime moves on each retry.
//   - ConsecutiveErrors: Number of consecutive failed fetches since the most
//     recent success. 0 when healthy. Useful as an alerting threshold.
type Status struct {
	Status             string `json:"status"`
	Uptime             string `json:"uptime"`
	LastFetchTime      string `json:"last_fetch_time"`
	LastFetchStatus    string `json:"last_fetch_status"`
	LastFetchSuccessAt string `json:"last_fetch_success_at"`
	ConsecutiveErrors  int    `json:"consecutive_errors"`
}

// Monitor tracks application health metrics.
//
// Thread-safety:
//   - All fields are protected by RWMutex
//   - Safe for concurrent updates from multiple goroutines
type Monitor struct {
	startTime          time.Time
	lastFetchTime      time.Time
	lastFetchStatus    string
	lastFetchSuccessAt time.Time
	consecutiveErrors  int
	mu                 sync.RWMutex
}

// NewMonitor creates a new health monitor.
//
// Initialization:
//   - Sets start time to current time
//   - Initializes status as empty (no fetches yet)
//
// Returns:
//   - *Monitor: Ready-to-use health monitor
func NewMonitor() *Monitor {
	return &Monitor{
		startTime:       time.Now(),
		lastFetchStatus: "not started",
	}
}

// UpdateFetchStatus updates the fetch status after a fetch attempt.
//
// This should be called:
//   - After successful fetch: UpdateFetchStatus("success")
//   - After failed fetch: UpdateFetchStatus("error: details")
//
// Thread-safety:
//   - Uses write lock for exclusive access
//
// Parameters:
//   - status: Status string ("success" or error message)
func (m *Monitor) UpdateFetchStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.lastFetchTime = now
	m.lastFetchStatus = status
	if status == "success" {
		m.lastFetchSuccessAt = now
		m.consecutiveErrors = 0
	} else {
		m.consecutiveErrors++
	}
}

// GetStatus returns the current health status.
//
// Thread-safety:
//   - Uses read lock for concurrent access
//
// Returns:
//   - Status: Current health status
func (m *Monitor) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uptime := time.Since(m.startTime)

	// Derive real health: "not started" is fine (startup phase),
	// "success" is healthy, anything else means the last fetch failed.
	overallStatus := "healthy"
	switch {
	case m.lastFetchStatus == "not started":
		overallStatus = "starting"
	case m.lastFetchStatus != "success":
		overallStatus = "unhealthy"
	}

	lastFetchTime := ""
	if !m.lastFetchTime.IsZero() {
		lastFetchTime = m.lastFetchTime.Format("2006-01-02 15:04:05")
	}
	lastFetchSuccessAt := ""
	if !m.lastFetchSuccessAt.IsZero() {
		lastFetchSuccessAt = m.lastFetchSuccessAt.Format("2006-01-02 15:04:05")
	}

	return Status{
		Status:             overallStatus,
		Uptime:             uptime.String(),
		LastFetchTime:      lastFetchTime,
		LastFetchStatus:    m.lastFetchStatus,
		LastFetchSuccessAt: lastFetchSuccessAt,
		ConsecutiveErrors:  m.consecutiveErrors,
	}
}

// registerStatusEndpoints wires /metrics (Prometheus-compatible) and /health
// (JSON status). Split out so tests can mount them on a httptest.Server
// without StartServer's WebSocket + listen loop.
func registerStatusEndpoints(mux *http.ServeMux, monitor *Monitor) {
	// Prometheus-compatible scrape endpoint. Counters and gauges are populated
	// by call-site instrumentation; cmon_open_complaints queries storage live
	// at scrape time.
	mux.Handle("/metrics", metrics.Handler())

	// JSON health endpoint for external probes. Returns 200 when healthy or
	// starting, 503 when unhealthy — so a probe can alert on HTTP code alone.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		s := monitor.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		if s.Status == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(s)
	})
}

// RefreshFunc is called by the dashboard to trigger a full scrape cycle
// before returning data. It should update storage with the latest complaints
// from the website. Returns nil on success.
type RefreshFunc func() error

// WSHub is the global WebSocket hub for real-time updates.
// It is initialized in StartServer and used by the dashboard.
var WSHub *Hub

// StartServer starts the local CMON dashboard server and returns the
// underlying *http.Server so the caller can drive a graceful Shutdown on
// process exit. The server itself runs in a background goroutine; this
// function does not block.
//
// Endpoints:
//   - GET /: Returns the pending complaints dashboard
//   - GET /data: Returns dashboard JSON data
//   - GET /ws: WebSocket endpoint for real-time updates
//   - GET /health: JSON health probe
//   - GET /metrics: Prometheus-compatible metrics
//
// Parameters:
//   - monitor: Health monitor to query for status
//   - port: Port to listen on (e.g., "8080")
//   - sc: Authenticated session client used by the complaints dashboard
//   - stor: Complaint storage used by the complaints dashboard
//   - refreshFn: Optional function to trigger a scrape cycle before returning data
func StartServer(monitor *Monitor, port string, sc *session.Client, stor *storage.Storage, refreshFn RefreshFunc) *http.Server {
	WSHub = NewHub()
	go WSHub.Run()

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, monitor, sc, stor, refreshFn)
	registerStatusEndpoints(mux, monitor)

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		WSHub.ServeHTTP(w, r)
	})

	srv := &http.Server{
		// Bind only to loopback — the dashboard has no authentication.
		// Expose it externally only via a reverse proxy with auth if needed.
		Addr:    "0.0.0.0:" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("✓ Dashboard server started on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("⚠️  Dashboard server error: %v", err)
		}
	}()

	return srv
}
