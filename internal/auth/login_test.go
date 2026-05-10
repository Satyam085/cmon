package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"cmon/internal/session"
)

// The auth package is a one-line shim around session.Client. The interesting
// behavior (captcha, login POST, session-expiry detection) lives in session
// and is tested there. These tests just lock in that the shim forwards
// correctly — i.e. a future refactor that breaks Login or IsSessionExpired
// will fail here, not silently in main.go.

func TestLoginShimForwardsToSessionClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<!doctype html><html><head>
<meta name="csrf-token" content="t">
</head><body>
<ul><li class="captchaList"><span>1 + 1</span></li></ul>
<input id="email_or_username">
</body></html>`)
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"shim-bearer"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sc, err := session.New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	if err := Login(sc, srv.URL+"/login", "u", "p"); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestIsSessionExpiredShimForwardsToSessionClient(t *testing.T) {
	expiredSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><input id="email_or_username"></body></html>`)
	}))
	defer expiredSrv.Close()

	liveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><h1>Dashboard</h1></body></html>`)
	}))
	defer liveSrv.Close()

	sc, err := session.New(1000, 1000, 0)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	if !IsSessionExpired(sc, expiredSrv.URL) {
		t.Error("expired page should report IsSessionExpired=true")
	}
	if IsSessionExpired(sc, liveSrv.URL) {
		t.Error("dashboard page should report IsSessionExpired=false")
	}
}
