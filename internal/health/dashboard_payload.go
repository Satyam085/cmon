package health

// Dashboard JSON payload builder + small response helpers used by the
// route handlers in dashboard_routes.go. Keeps the JSON-shaping logic
// separate from the HTML template (complaints.go) and the export
// flattener (dashboard_export.go).

import (
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"strings"
	"time"

	"cmon/internal/belt"
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
			GroupCount:  0,
			Status:      status,
			Groups:      []complaintGroupPayload{},
		}, nil
	}

	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		if strings.Contains(err.Error(), "no pending complaints found") || strings.Contains(err.Error(), "no complaints with valid API IDs") {
			return complaintDashboardPayload{
				GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
				TotalCount:  0,
				GroupCount:  0,
				Status:      status,
				Groups:      []complaintGroupPayload{},
			}, nil
		}
		return complaintDashboardPayload{}, fmt.Errorf("failed to fetch pending complaints: %w", err)
	}

	grouped := summary.GroupComplaints(complaints)
	groups := make([]complaintGroupPayload, 0, len(grouped))
	totalCount := 0
	for _, group := range grouped {
		style := belt.StyleFor(group.Belt)
		totalCount += len(group.Complaints)
		groups = append(groups, complaintGroupPayload{
			Belt:       belt.DisplayName(group.Belt),
			Label:      style.Label,
			Emoji:      style.Emoji,
			Count:      len(group.Complaints),
			FillColor:  colorToHex(style.Fill),
			TextColor:  colorToHex(style.Text),
			Complaints: group.Complaints,
		})
	}

	return complaintDashboardPayload{
		GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
		TotalCount:  totalCount,
		GroupCount:  len(groups),
		Status:      status,
		Groups:      groups,
	}, nil
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
