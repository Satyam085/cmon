package main

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// ResolveComplaintOnWebsite marks a complaint as resolved on the DGVCL website using the browser context
func ResolveComplaintOnWebsite(ctx context.Context, apiID string, remark string, debugMode bool) error {
	apiURL := "https://complaint.dgvcl.com/api/complaint-assign-process"
	
	// URL encode the remark
	encodedRemark := url.QueryEscape(remark)
	
	// Build the request body
	requestBody := fmt.Sprintf("complaint_id=%s&complaint_AsignType=resolved&remark=%s", apiID, encodedRemark)
	
	log.Printf("  → Marking complaint %s as resolved on website...\n", apiID)
	
	// Debug mode: Just log instead of making API call
	if debugMode {
		log.Printf("  🐛 DEBUG MODE: Skipping API call\n")
		log.Printf("  🐛 Would call: %s\n", apiURL)
		log.Printf("  🐛 With body: %s\n", requestBody)
		log.Printf("  ✓ [DEBUG] Simulated successful resolution\n")
		return nil
	}
	
	var responseText string
	err := chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				try {
					const response = await fetch('%s', {
						method: 'POST',
						headers: {
							'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
							'X-Requested-With': 'XMLHttpRequest'
						},
						body: '%s'
					});
					if (!response.ok) throw new Error('HTTP status ' + response.status);
					return await response.text();
				} catch (error) {
					return 'ERROR: ' + error.message;
				}
			})()
		`, apiURL, requestBody), &responseText, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	
	if err != nil {
		return fmt.Errorf("failed to execute API call: %w", err)
	}
	
	if responseText != "" && responseText[:6] == "ERROR:" {
		return fmt.Errorf("API call failed: %s", responseText[7:])
	}
	
	log.Printf("  ✓ Successfully marked complaint %s as resolved on website\n", apiID)
	log.Printf("  → API Response: %s\n", responseText)
	
	return nil
}
