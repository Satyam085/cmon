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

// complaintRequest holds the mapping between a complaint number and its API ID.
type complaintRequest struct {
	complaintNo string
	apiID       string
}

// FetchAllPendingDetails fetches complaint details for all pending complaints.
//
// It batches all API calls into a single browser round-trip using JavaScript
// Promise.all(), making all fetches run concurrently. This is significantly
// faster than fetching one complaint at a time.
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

	// Build list of valid complaint requests (those with API IDs)
	var requests []complaintRequest
	for _, id := range complaintIDs {
		apiID := stor.GetAPIID(id)
		if apiID == "" {
			log.Printf("  ⚠️  No API ID for complaint %s, skipping", id)
			continue
		}
		requests = append(requests, complaintRequest{complaintNo: id, apiID: apiID})
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("no complaints with valid API IDs")
	}

	// Try batch fetch first (all concurrent via Promise.all)
	complaints, err := fetchAllBatch(ctx, requests)
	if err != nil {
		log.Printf("  ⚠️  Batch fetch failed (%v), falling back to serial fetch", err)
		complaints = fetchAllSerial(ctx, requests)
	}

	if len(complaints) == 0 {
		return nil, fmt.Errorf("failed to fetch any complaint details")
	}

	log.Printf("📊 Successfully fetched %d/%d complaint details", len(complaints), len(requests))
	return complaints, nil
}

// fetchAllBatch fetches all complaints concurrently using a single chromedp.Run
// with JavaScript Promise.allSettled(). Each API call runs in parallel within the
// browser's JS engine, and results are returned as a JSON array.
func fetchAllBatch(ctx context.Context, requests []complaintRequest) ([]Complaint, error) {
	// Build JSON array of API URLs for JavaScript
	type fetchItem struct {
		URL         string `json:"url"`
		ComplaintNo string `json:"complaintNo"`
	}
	items := make([]fetchItem, len(requests))
	for i, r := range requests {
		items[i] = fetchItem{
			URL:         fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", r.apiID),
			ComplaintNo: r.complaintNo,
		}
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fetch items: %w", err)
	}

	// JavaScript that fetches all URLs concurrently using Promise.allSettled
	// and returns an array of {status, complaintNo, data} objects.
	js := fmt.Sprintf(`
		(async function() {
			const items = %s;
			const results = await Promise.allSettled(
				items.map(async (item) => {
					const response = await fetch(item.url, {
						headers: { 'X-Requested-With': 'XMLHttpRequest' }
					});
					if (!response.ok) throw new Error('HTTP status ' + response.status);
					const data = await response.json();
					return { complaintNo: item.complaintNo, data: data };
				})
			);
			// Return fulfilled results with their complaint numbers
			return JSON.stringify(
				results
					.filter(r => r.status === 'fulfilled')
					.map(r => r.value)
			);
		})()
	`, string(itemsJSON))

	var jsonResponse string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(js, &jsonResponse, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("batch fetch failed: %w", err)
	}

	// Parse the JSON array of results
	var batchResults []struct {
		ComplaintNo string                 `json:"complaintNo"`
		Data        map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(jsonResponse), &batchResults); err != nil {
		return nil, fmt.Errorf("batch JSON parse failed: %w", err)
	}

	var complaints []Complaint
	for _, result := range batchResults {
		detail, ok := result.Data["complaintdetail"].(map[string]interface{})
		if !ok {
			log.Printf("  ⚠️  complaintdetail missing for %s", result.ComplaintNo)
			continue
		}

		complaints = append(complaints, Complaint{
			ComplainNo:   safeStr(detail["complain_no"]),
			Name:         safeStr(detail["complainant_name"]),
			ConsumerNo:   safeStr(detail["consumer_no"]),
			MobileNo:     safeStr(detail["mobile_no"]),
			Address:      safeStr(detail["exact_location"]),
			Area:         safeStr(detail["area"]),
			Description:  safeStr(detail["description"]),
			ComplainDate: safeStr(detail["complain_date"]),
		})
	}

	return complaints, nil
}

// fetchAllSerial is the fallback that fetches complaints one at a time.
// Used when the batch approach fails (e.g., browser issues).
func fetchAllSerial(ctx context.Context, requests []complaintRequest) []Complaint {
	var complaints []Complaint
	for _, r := range requests {
		c, err := fetchComplaintDetail(ctx, r.apiID, r.complaintNo)
		if err != nil {
			log.Printf("  ⚠️  Failed to fetch details for %s: %v", r.complaintNo, err)
			continue
		}
		complaints = append(complaints, *c)
	}
	return complaints
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
