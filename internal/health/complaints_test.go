package health

import (
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

	stor := storage.New()
	t.Cleanup(func() {
		_ = stor.Close()
	})

	mux := http.NewServeMux()
	registerComplaintDashboard(mux, NewMonitor(), nil, stor)

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
		if !strings.Contains(body, `const dataUrl = "/data";`) {
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
