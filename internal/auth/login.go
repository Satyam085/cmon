// Package auth handles authentication for the DGVCL portal.
//
// Login is now delegated entirely to the session.Client, which uses
// standard net/http + goquery instead of a headless Chrome browser.
// This package is kept as a thin shim for backwards compatibility with
// main.go call sites.
package auth

import (
	"cmon/internal/session"
)

// Login authenticates with the DGVCL portal using the provided session client.
//
// The session client fetches the login page, parses the arithmetic captcha
// with goquery, and submits the credentials via HTTP POST. The resulting
// session cookie is stored in the client's cookie jar automatically.
//
// Parameters:
//   - sc: Authenticated session client
//   - loginURL: URL of the login page
//   - username: DGVCL portal username
//   - password: DGVCL portal password
//
// Returns:
//   - error: LoginFailedError if login fails, nil on success
func Login(sc *session.Client, loginURL, username, password string) error {
	return sc.Login(loginURL, username, password)
}

// IsSessionExpired checks if the authenticated session is still valid.
//
// Parameters:
//   - sc: Session client to check
//   - dashboardURL: URL of the authenticated area to probe
//
// Returns:
//   - bool: true if session expired/invalid, false if still active
func IsSessionExpired(sc *session.Client, dashboardURL string) bool {
	return sc.IsSessionExpired(dashboardURL)
}
