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
)

// Status represents the application health status.
//
// This is returned by the /health endpoint for monitoring tools.
//
// Fields:
//   - Status: Overall health status ("healthy" or "unhealthy")
//   - Uptime: How long the application has been running
//   - LastFetchTime: When the last complaint fetch completed
//   - LastFetchStatus: Status of last fetch ("success" or error message)
type Status struct {
	Status          string `json:"status"`
	Uptime          string `json:"uptime"`
	LastFetchTime   string `json:"last_fetch_time"`
	LastFetchStatus string `json:"last_fetch_status"`
}

// Monitor tracks application health metrics.
//
// Thread-safety:
//   - All fields are protected by RWMutex
//   - Safe for concurrent updates from multiple goroutines
//
// Fields:
//   - startTime: When the application started
//   - lastFetchTime: When the last fetch completed
//   - lastFetchStatus: Status of the last fetch
//   - mu: Mutex for thread-safe access
type Monitor struct {
	startTime       time.Time
	lastFetchTime   time.Time
	lastFetchStatus string
	mu              sync.RWMutex
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
	m.lastFetchTime = time.Now()
	m.lastFetchStatus = status
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

	return Status{
		Status:          "healthy",
		Uptime:          uptime.String(),
		LastFetchTime:   m.lastFetchTime.Format("2006-01-02 15:04:05"),
		LastFetchStatus: m.lastFetchStatus,
	}
}

// StartServer starts the health check HTTP server.
//
// Endpoints:
//   - GET /health: Returns JSON health status
//
// Server runs in background goroutine and doesn't block.
//
// Example response:
//   {
//     "status": "healthy",
//     "uptime": "1h2m3s",
//     "last_fetch_time": "2026-01-15 10:30:00",
//     "last_fetch_status": "success"
//   }
//
// Parameters:
//   - monitor: Health monitor to query for status
//   - port: Port to listen on (e.g., "8080")
func StartServer(monitor *Monitor, port string) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := monitor.GetStatus()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(status)
	})

	go func() {
		log.Printf("✓ Health check server started on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("⚠️  Health check server error: %v", err)
		}
	}()
}
