package health

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cmon/internal/sfms"
)

func TestSFMSRoutes(t *testing.T) {
	cfg := &sfms.Config{
		BearerToken: "Bearer test-token",
	}
	client := sfms.NewClient(cfg)
	mon := sfms.NewMonitor(cfg, client, nil, nil, nil, nil)

	mux := http.NewServeMux()
	registerSFMSDashboard(mux, mon)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. GET /sfms
	resp, err := http.Get(srv.URL + "/sfms")
	if err != nil {
		t.Fatalf("GET /sfms error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /sfms status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. GET /sfms/data
	resp, err = http.Get(srv.URL + "/sfms/data")
	if err != nil {
		t.Fatalf("GET /sfms/data error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /sfms/data status = %d, want 200", resp.StatusCode)
	}
	var payload sfms.DashboardPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload error: %v", err)
	}
	resp.Body.Close()
	if !payload.TokenActive {
		t.Errorf("TokenActive should be true")
	}

	// 3. POST /sfms/update-token with empty token -> should fail with 400
	reqBody, _ := json.Marshal(map[string]string{"token": ""})
	resp, err = http.Post(srv.URL+"/sfms/update-token", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /sfms/update-token error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty token update should return 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. POST /sfms/refresh
	resp, err = http.Post(srv.URL+"/sfms/refresh", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /sfms/refresh error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /sfms/refresh status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
