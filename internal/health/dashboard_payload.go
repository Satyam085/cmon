package health

// Dashboard JSON payload builder + small response helpers used by the
// route handlers in dashboard_routes.go. Keeps the JSON-shaping logic
// separate from the HTML template (complaints.go) and the export
// flattener (dashboard_export.go).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/summary"
)

func buildComplaintDashboardPayload(monitor *Monitor, sc *session.Client, stor *storage.Storage) (complaintDashboardPayload, error) {
	status := monitor.GetStatus()
	activeIDs := stor.GetAllSeenComplaints()
	if len(activeIDs) == 0 {
		return complaintDashboardPayload{
			GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
			TotalCount:  0,
			Status:      status,
			Complaints:  []summary.Complaint{},
		}, nil
	}

	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		if strings.Contains(err.Error(), "no pending complaints found") || strings.Contains(err.Error(), "no complaints with valid API IDs") {
			return complaintDashboardPayload{
				GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
				TotalCount:  0,
				Status:      status,
				Complaints:  []summary.Complaint{},
			}, nil
		}
		return complaintDashboardPayload{}, fmt.Errorf("failed to fetch pending complaints: %w", err)
	}

	sorted := summary.SortComplaints(complaints)

	return complaintDashboardPayload{
		GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
		TotalCount:  len(sorted),
		Status:      status,
		Complaints:  sorted,
	}, nil
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
