// Package api provides external API interaction for the CMON application.
package api

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// ResolveComplaint marks a complaint as resolved on the DGVCL website.
//
// This function uses the browser context to make an authenticated API call
// to the DGVCL complaint resolution endpoint. Using the browser context
// ensures that session cookies are automatically included in the request.
//
// API Details:
//   - Endpoint: https://complaint.dgvcl.com/api/complaint-assign-process
//   - Method: POST
//   - Content-Type: application/x-www-form-urlencoded
//   - Authentication: Via session cookies (from browser context)
//
// Request body format:
//   complaint_id=<apiID>&complaint_AsignType=resolved&remark=<encoded_remark>
//
// Flow:
//   1. URL-encode the remark text
//   2. Build request body with complaint ID and remark
//   3. Execute fetch() call inside browser context
//   4. Wait for response using async/await
//   5. Return response text or error
//
// Debug mode:
//   - When enabled, logs the API call details without making actual request
//   - Useful for testing without modifying production data
//
// Parameters:
//   - ctx: Browser context (contains session cookies)
//   - apiID: Internal complaint ID from the API
//   - remark: Resolution note/comment from user
//   - debugMode: If true, simulate the call without executing
//
// Returns:
//   - error: API call failure or HTTP error, nil on success
func ResolveComplaint(ctx context.Context, apiID string, remark string, debugMode bool) error {
	apiURL := "https://complaint.dgvcl.com/api/complaint-assign-process"

	// URL-encode the remark to handle special characters
	// Example: "Fixed & tested" → "Fixed+%26+tested"
	encodedRemark := url.QueryEscape(remark)

	// Build request body in application/x-www-form-urlencoded format
	// Format: key1=value1&key2=value2&key3=value3
	requestBody := fmt.Sprintf("complaint_id=%s&complaint_AsignType=resolved&remark=%s", apiID, encodedRemark)

	log.Printf("  → Marking complaint %s as resolved on website...\n", apiID)

	// Debug mode: Log without executing
	if debugMode {
		log.Printf("  🐛 DEBUG MODE: Skipping API call\n")
		log.Printf("  🐛 Would call: %s\n", apiURL)
		log.Printf("  🐛 With body: %s\n", requestBody)
		log.Printf("  ✓ [DEBUG] Simulated successful resolution\n")
		return nil
	}

	// Execute API call inside browser context
	// This ensures session cookies are included automatically
	var responseText string

	// Use chromedp.Evaluate to run JavaScript in the browser
	// The async/await pattern is crucial here:
	//   - fetch() returns a Promise
	//   - await ensures we wait for the response
	//   - WithAwaitPromise(true) tells ChromeDP to wait for Promise resolution
	//
	// Without WithAwaitPromise(true), chromedp would try to unmarshal the
	// pending Promise object into a string, causing a type error.
	err := chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				try {
					// Make POST request with session cookies
					const response = await fetch('%s', {
						method: 'POST',
						headers: {
							'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
							'X-Requested-With': 'XMLHttpRequest'
						},
						body: '%s'
					});
					
					// Check HTTP status
					if (!response.ok) throw new Error('HTTP status ' + response.status);
					
					// Return response text
					return await response.text();
				} catch (error) {
					// Return error with prefix for detection
					return 'ERROR: ' + error.message;
				}
			})()
		`, apiURL, requestBody), &responseText, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			// Wait for the async function to complete
			return p.WithAwaitPromise(true)
		}),
	)

	// Check for JavaScript execution error
	if err != nil {
		return fmt.Errorf("failed to execute API call: %w", err)
	}

	// Check for API error (returned as "ERROR: message")
	if len(responseText) >= 6 && responseText[:6] == "ERROR:" {
		return fmt.Errorf("API call failed: %s", responseText[7:])
	}

	log.Printf("  ✓ Successfully marked complaint %s as resolved on website\n", apiID)
	log.Printf("  → API Response: %s\n", responseText)

	return nil
}
