package health

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cmon/internal/storage"
)

func withTempCWD(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func TestDashboardRoutes(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	t.Run("root serves dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET / returned %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if body == "" {
			t.Fatal("GET / returned empty body")
		}
		if !strings.Contains(body, `const DATA_URL = "/data";`) {
			t.Fatalf("dashboard page did not embed expected data URL, body was: %s", body)
		}
	})

	t.Run("data serves json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/data", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /data returned %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("legacy complaints route redirects", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/complaints", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusPermanentRedirect {
			t.Fatalf("GET /complaints returned %d, want %d", rec.Code, http.StatusPermanentRedirect)
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Fatalf("GET /complaints redirected to %q, want %q", got, "/")
		}
	})
}

// seedExportFixtures loads two complete complaint records from two different
// belts so the export tests have something to flatten. Records are saved
// with every detail field populated so `summary.needsRefetch` returns false
// and the dashboard payload builder never tries to hit the (nil) session
// client for backfill.
func seedExportFixtures(t *testing.T, stor *storage.Storage) {
	t.Helper()
	if err := stor.SaveMultiple([]storage.Record{
		{
			ComplaintID:  "C-1",
			APIID:        "API-1",
			ConsumerName: "Alice",
			Village:      "Tokarva",
			Belt:         "Bajipura",
			ConsumerNo:   "CONS-0001",
			MobileNo:     "9000000001",
			Address:      "House 1",
			Area:         "Area-A",
			Description:  "LITE NATHI",
			ComplainDate: "2026-05-01 10:00:00",
		},
		{
			ComplaintID:  "C-2",
			APIID:        "API-2",
			ConsumerName: "Bob, with comma",
			Village:      "Some Village",
			Belt:         "Dahod",
			ConsumerNo:   "CONS-0002",
			MobileNo:     "9000000002",
			Address:      "House 2",
			Area:         "Area-B",
			Description:  "TC \"burnt\"",
			ComplainDate: "2026-05-02 11:00:00",
		},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestExportJSONReturnsFlatList(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	seedExportFixtures(t, stor)

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	req := httptest.NewRequest(http.MethodGet, "/export.json", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /export.json returned %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json prefix", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="cmon-complaints-`) {
		t.Errorf("Content-Disposition: got %q, want attachment with date-stamped filename", cd)
	}

	var payload struct {
		GeneratedAt string      `json:"generated_at"`
		TotalCount  int         `json:"total_count"`
		Complaints  []exportRow `json:"complaints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, rec.Body.String())
	}

	if payload.TotalCount != 2 {
		t.Errorf("total_count: got %d, want 2", payload.TotalCount)
	}
	if len(payload.Complaints) != 2 {
		t.Fatalf("complaints len: got %d, want 2", len(payload.Complaints))
	}

	// Row contents: every operationally useful field must be present.
	byID := map[string]exportRow{}
	for _, r := range payload.Complaints {
		byID[r.ComplainNo] = r
	}
	c1, ok := byID["C-1"]
	if !ok {
		t.Fatal("expected complaint C-1 in export")
	}
	if c1.Belt == "" || c1.Description == "" || c1.MobileNo == "" {
		t.Errorf("C-1 missing operational fields: %+v", c1)
	}
}

func TestExportJSONFiltersByBeltQueryParam(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	seedExportFixtures(t, stor)

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	// First read /data to learn the canonical belt display name the dashboard
	// uses — exporting via that same key is the contract we want to lock in.
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /data: %d body=%s", rec.Code, rec.Body.String())
	}
	var dash struct {
		Groups []struct {
			Belt string `json:"belt"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dash); err != nil {
		t.Fatalf("decode /data: %v", err)
	}
	if len(dash.Groups) == 0 {
		t.Fatal("expected at least one belt group in /data")
	}
	wantBelt := dash.Groups[0].Belt

	// Now export, filtered to that belt.
	req = httptest.NewRequest(http.MethodGet, "/export.json?belt="+wantBelt, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("filtered export: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Complaints []exportRow `json:"complaints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Complaints) == 0 {
		t.Fatal("filtered export returned 0 rows; expected at least one")
	}
	for _, r := range payload.Complaints {
		if r.Belt != wantBelt {
			t.Errorf("belt filter leaked row from %q (expected only %q)", r.Belt, wantBelt)
		}
	}
}

func TestVillagesEndpointReturnsBreakdown(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	// Two complaints in Bajipura belt, one in Dahod, with three distinct
	// villages so we can verify the per-belt scoping.
	if err := stor.SaveMultiple([]storage.Record{
		{ComplaintID: "C-1", APIID: "A-1", Belt: "bajipura", Village: "Tokarva"},
		{ComplaintID: "C-2", APIID: "A-2", Belt: "bajipura", Village: "Tokarva"},
		{ComplaintID: "C-3", APIID: "A-3", Belt: "bajipura", Village: "Bajipura"},
		{ComplaintID: "C-4", APIID: "A-4", Belt: "dahod", Village: "Dahod"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	req := httptest.NewRequest(http.MethodGet, "/villages?belt=bajipura", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /villages: %d body=%s", rec.Code, rec.Body.String())
	}

	var out struct {
		Belt     string `json:"belt"`
		Total    int    `json:"total"`
		Villages []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"villages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if out.Total != 3 {
		t.Errorf("total: got %d, want 3 (only Bajipura belt)", out.Total)
	}
	if len(out.Villages) != 2 {
		t.Fatalf("villages count: got %d, want 2 (Tokarva + Bajipura)", len(out.Villages))
	}
	// Sort contract: descending by count, then alphabetical.
	if out.Villages[0].Name != "Tokarva" || out.Villages[0].Count != 2 {
		t.Errorf("first village: got %+v, want {Tokarva 2}", out.Villages[0])
	}
	if out.Villages[1].Name != "Bajipura" || out.Villages[1].Count != 1 {
		t.Errorf("second village: got %+v, want {Bajipura 1}", out.Villages[1])
	}
}

func TestVillagesEndpointAcceptsDisplayName(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	if err := stor.SaveMultiple([]storage.Record{
		{ComplaintID: "C-1", APIID: "A-1", Belt: "bajipura", Village: "Tokarva"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	// The dashboard tabs use the display name, so the URL has to accept that.
	// belt.Canonicalize lowercases and matches; "Bajipura" should resolve.
	req := httptest.NewRequest(http.MethodGet, "/villages?belt=Bajipura", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /villages?belt=Bajipura: %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 1 {
		t.Errorf("display-name lookup should hit the same data as canonical; got total=%d, want 1", out.Total)
	}
}

func TestVillagesEndpointMissingBeltIs400(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	req := httptest.NewRequest(http.MethodGet, "/villages", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing belt: got %d, want 400", rec.Code)
	}
}

func TestExportCSVMatchesHeaderAndQuotesCorrectly(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = stor.Close() })

	seedExportFixtures(t, stor)

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor, nil)

	req := httptest.NewRequest(http.MethodGet, "/export.csv", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /export.csv: %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type: got %q, want text/csv prefix", ct)
	}

	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV reader rejected output: %v", err)
	}
	if len(rows) != 1+2 {
		t.Fatalf("rows: got %d, want 1 header + 2 data rows", len(rows))
	}

	// Header must match the package-level slice exactly — drift between the
	// two would silently change every export consumer.
	for i, col := range exportCSVHeader {
		if rows[0][i] != col {
			t.Errorf("header col %d: got %q, want %q", i, rows[0][i], col)
		}
	}
	if len(rows[0]) != len(exportCSVHeader) {
		t.Errorf("header column count: got %d, want %d", len(rows[0]), len(exportCSVHeader))
	}

	// The Bob row carries a comma in the consumer name and a literal double
	// quote in the description — encoding/csv handles both. Round-tripping
	// through encoding/csv.ReadAll above already proves it; this row-level
	// check makes a regression in either field obvious in the failure log.
	var bob []string
	for _, row := range rows[1:] {
		if row[1] == "C-2" { // index 1 = complain_no per header
			bob = row
			break
		}
	}
	if bob == nil {
		t.Fatal("did not find C-2 row in CSV")
	}
	if bob[2] != "Bob, with comma" {
		t.Errorf("name with comma round-trip: got %q, want %q", bob[2], "Bob, with comma")
	}
	if bob[8] != `TC "burnt"` { // index 8 = description per header
		t.Errorf("description with quotes round-trip: got %q, want %q", bob[8], `TC "burnt"`)
	}
}
