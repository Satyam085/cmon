// Package summary handles generating summary images for pending complaints.
package summary

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"cmon/internal/storage"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// FetchAllPendingDetails fetches complaint details for all pending complaints.
//
// It iterates over all complaints in storage, fetches their details from the
// DGVCL API using the browser context (for session cookies), and returns
// a slice of Complaint structs ready for rendering.
//
// Parameters:
//   - ctx: Browser context with authenticated session
//   - stor: Storage containing pending complaint IDs and API IDs
//
// Returns:
//   - []Complaint: Complaint details for all pending complaints
//   - error: If no complaints found or all fetches fail
func FetchAllPendingDetails(ctx context.Context, stor *storage.Storage) ([]Complaint, error) {
	complaintIDs := stor.GetAllSeenComplaints()
	if len(complaintIDs) == 0 {
		return nil, fmt.Errorf("no pending complaints found")
	}

	log.Printf("📊 Fetching details for %d pending complaints...", len(complaintIDs))

	var complaints []Complaint
	for _, id := range complaintIDs {
		apiID := stor.GetAPIID(id)
		if apiID == "" {
			log.Printf("  ⚠️  No API ID for complaint %s, skipping", id)
			continue
		}

		c, err := fetchComplaintDetail(ctx, apiID, id)
		if err != nil {
			log.Printf("  ⚠️  Failed to fetch details for %s: %v", id, err)
			continue
		}

		complaints = append(complaints, *c)
	}

	if len(complaints) == 0 {
		return nil, fmt.Errorf("failed to fetch any complaint details")
	}

	log.Printf("📊 Successfully fetched %d/%d complaint details", len(complaints), len(complaintIDs))
	return complaints, nil
}

// fetchComplaintDetail fetches a single complaint's details from the API.
func fetchComplaintDetail(ctx context.Context, apiID, complaintNumber string) (*Complaint, error) {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", apiID)

	var jsonResponse string
	err := chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				const response = await fetch('%s', {
					headers: { 'X-Requested-With': 'XMLHttpRequest' }
				});
				if (!response.ok) throw new Error('HTTP status ' + response.status);
				return await response.text();
			})()
		`, apiURL), &jsonResponse, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	// Parse JSON
	var fullData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResponse), &fullData); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}

	detail, ok := fullData["complaintdetail"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("complaintdetail missing in response")
	}

	return &Complaint{
		ComplainNo:   safeStr(detail["complain_no"]),
		Name:         safeStr(detail["complainant_name"]),
		ConsumerNo:   safeStr(detail["consumer_no"]),
		MobileNo:     safeStr(detail["mobile_no"]),
		Address:      safeStr(detail["exact_location"]),
		Area:         safeStr(detail["area"]),
		Description:  safeStr(detail["description"]),
		ComplainDate: safeStr(detail["complain_date"]),
	}, nil
}

// safeStr converts an interface{} value to string, handling nil.
func safeStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
