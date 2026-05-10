// Package api provides external API interaction for the CMON application.
package api

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"cmon/internal/metrics"
	"cmon/internal/session"
)

// DefaultResolveEndpoint is the DGVCL production URL used by ResolveComplaint
// when no override has been installed. main.go can override this at boot via
// SetResolveEndpoint so a staging environment can point at its own backend.
const DefaultResolveEndpoint = "https://complaint.dgvcl.com/api/complaint-assign-process"

// resolveEndpoint is the active endpoint URL. Mutated only from
// SetResolveEndpoint (boot-time, single-threaded) and from package tests.
var resolveEndpoint = DefaultResolveEndpoint

// SetResolveEndpoint installs the URL ResolveComplaint posts to. Intended for
// boot-time initialisation from config; passing an empty string is a no-op
// so accidental config gaps don't blank-out the endpoint.
func SetResolveEndpoint(url string) {
	if url == "" {
		return
	}
	resolveEndpoint = url
}

// ResolveComplaint marks a complaint as resolved on the DGVCL website.
//
// This function uses the session client to make an authenticated HTTP POST
// to the DGVCL complaint resolution endpoint. The cookie jar in the session
// client automatically includes the session cookie — exactly what the browser
// context was doing before.
//
// API Details:
//   - Endpoint: https://complaint.dgvcl.com/api/complaint-assign-process
//   - Method: POST
//   - Content-Type: application/x-www-form-urlencoded
//   - Authentication: Via session cookies (from cookie jar)
//
// Request body format:
//
//	complaint_id=<apiID>&complaint_AsignType=resolved&remark=<encoded_remark>
//
// Parameters:
//   - sc: Authenticated session client (contains cookies)
//   - apiID: Internal complaint ID from the API
//   - remark: Resolution note/comment from user
//   - debugMode: If true, simulate the call without executing
//
// Returns:
//   - error: API call failure or HTTP error, nil on success
func ResolveComplaint(sc *session.Client, apiID string, remark string, debugMode bool) error {
	apiURL := resolveEndpoint

	formData := url.Values{
		"complaint_id":        {apiID},
		"complaint_AsignType": {"resolved"},
		"remark":              {remark},
	}

	log.Printf("  → Marking complaint %s as resolved on website...\n", apiID)

	if debugMode {
		log.Printf("  🐛 DEBUG MODE: Skipping API call\n")
		log.Printf("  🐛 Would POST: %s\n", apiURL)
		log.Printf("  🐛 With body: %s\n", formData.Encode())
		log.Printf("  ✓ [DEBUG] Simulated successful resolution\n")
		return nil
	}

	metrics.ResolveCallsTotal.Inc()
	responseBody, err := sc.PostForm(apiURL, formData)
	if err != nil {
		metrics.ResolveFailuresTotal.Inc()
		return fmt.Errorf("failed to execute API call: %w", err)
	}

	responseText := strings.TrimSpace(string(responseBody))

	// Check for API error
	if strings.HasPrefix(responseText, "ERROR:") {
		metrics.ResolveFailuresTotal.Inc()
		return fmt.Errorf("API call failed: %s", responseText[6:])
	}

	log.Printf("  ✓ Successfully marked complaint %s as resolved on website\n", apiID)
	log.Printf("  → API Response: %s\n", responseText)

	return nil
}
