package health

// This file owns HTTP route registration for the dashboard mux. The
// rendering template lives in complaints.go; payload + helpers in
// dashboard_payload.go; export rows in dashboard_export.go.

import (
	"encoding/csv"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"cmon/internal/api"
	"cmon/internal/belt"
	"cmon/internal/session"
	"cmon/internal/storage"
)

func registerComplaintDashboard(mux *http.ServeMux, monitor *Monitor, sc *session.Client, stor *storage.Storage, refreshFn RefreshFunc) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		beltsJSON, _ := json.Marshal(belt.All())

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = complaintsPageTemplate.Execute(w, complaintDashboardPageData{
			DataURL:   "/data",
			BeltsJSON: template.JS(beltsJSON),
		})
	})

	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		payload, err := buildComplaintDashboardPayload(monitor, sc, stor)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if refreshFn == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "refresh not available")
			return
		}

		if err := refreshFn(); err != nil {
			log.Printf("⚠️  Dashboard-triggered scrape failed: %v", err)
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// /resolve — mark a complaint as resolved on the DGVCL portal
	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req struct {
			ComplaintID string `json:"complaint_id"`
			Remark      string `json:"remark"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.ComplaintID == "" {
			writeJSONError(w, http.StatusBadRequest, "complaint_id is required")
			return
		}

		remark := req.Remark
		if remark == "" {
			remark = "Resolved via dashboard"
		}

		log.Printf("🌐 Dashboard: resolving complaint API ID %s (remark: %q)", req.ComplaintID, remark)
		if err := api.ResolveComplaint(sc, req.ComplaintID, remark, false); err != nil {
			log.Printf("⚠️  Dashboard resolve failed for %s: %v", req.ComplaintID, err)
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		if WSHub != nil {
			WSHub.BroadcastResolved(req.ComplaintID)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// /move — reassign a complaint to a different belt (dashboard equivalent
	// of the Telegram /move command). Keyed by complaint number, not API ID.
	mux.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req struct {
			ComplaintID string `json:"complaint_id"`
			Belt        string `json:"belt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.ComplaintID == "" {
			writeJSONError(w, http.StatusBadRequest, "complaint_id is required")
			return
		}

		newBelt, ok := belt.Canonicalize(req.Belt)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "unknown belt: "+req.Belt)
			return
		}
		if !stor.Exists(req.ComplaintID) {
			writeJSONError(w, http.StatusBadRequest, "complaint not in active storage")
			return
		}

		if err := stor.UpdateBelt(req.ComplaintID, newBelt); err != nil {
			log.Printf("⚠️  Dashboard move failed for %s: %v", req.ComplaintID, err)
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		log.Printf("🌐 Dashboard: moved complaint %s to belt %s", req.ComplaintID, newBelt)
		if WSHub != nil {
			WSHub.BroadcastRefresh()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "belt": newBelt})
	})

	// /villages returns the village→count breakdown for one belt. Powers the
	// dashboard drill-down: clicking a belt's "Villages" badge fetches this
	// endpoint and surfaces the breakdown without re-running the full
	// dashboard payload pipeline.
	mux.HandleFunc("/villages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		input := strings.TrimSpace(r.URL.Query().Get("belt"))
		if input == "" {
			writeJSONError(w, http.StatusBadRequest, "belt query parameter is required")
			return
		}

		// Accept either a display name (what the dashboard tab uses) or a
		// canonical key. Canonicalize first; fall back to raw input so an
		// unknown name still produces a (probably empty) result instead of
		// a confusing 400.
		canonical := input
		if c, ok := belt.Canonicalize(input); ok {
			canonical = c
		}

		counts := stor.GetVillageCountsByBelt(canonical)
		villages := make([]map[string]interface{}, 0, len(counts))
		total := 0
		for name, count := range counts {
			villages = append(villages, map[string]interface{}{
				"name":  name,
				"count": count,
			})
			total += count
		}
		// Stable order: descending count, then alphabetical. Operator scans
		// top to bottom looking for the worst-affected village.
		sort.Slice(villages, func(i, j int) bool {
			ci := villages[i]["count"].(int)
			cj := villages[j]["count"].(int)
			if ci != cj {
				return ci > cj
			}
			return villages[i]["name"].(string) < villages[j]["name"].(string)
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"belt":     belt.DisplayName(canonical),
			"total":    total,
			"villages": villages,
		})
	})

	// /export.json and /export.csv emit a flat list of currently-pending
	// complaints for audits and ad-hoc analysis. Both reuse the same
	// dashboard payload builder, then flatten the belt-grouped structure
	// into a per-row form. Optional ?belt=<display-name> scopes the export
	// to a single belt — matches the dashboard tab key.
	mux.HandleFunc("/export.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		rows, generatedAt, err := buildExportRows(monitor, sc, stor, r.URL.Query().Get("belt"))
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename("json")+`"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"generated_at": generatedAt,
			"total_count":  len(rows),
			"complaints":   rows,
		})
	})

	mux.HandleFunc("/export.csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rows, _, err := buildExportRows(monitor, sc, stor, r.URL.Query().Get("belt"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename("csv")+`"`)
		w.WriteHeader(http.StatusOK)

		cw := csv.NewWriter(w)
		_ = cw.Write(exportCSVHeader)
		for _, row := range rows {
			_ = cw.Write(row.csvRecord())
		}
		cw.Flush()
	})

	mux.HandleFunc("/complaints", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	})

	mux.HandleFunc("/complaints/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/complaints/", "/complaints":
			http.Redirect(w, r, "/", http.StatusPermanentRedirect)
		case "/complaints/data", "/complaints/data/":
			http.Redirect(w, r, "/data", http.StatusPermanentRedirect)
		default:
			http.NotFound(w, r)
		}
	})
}
