// Package summary handles generating summary images for pending complaints.
package summary

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"cmon/internal/session"
	"cmon/internal/storage"
)

// complaintRequest holds the mapping between a complaint number and its API ID.
type complaintRequest struct {
	complaintNo string
	apiID       string
}

// FetchAllPendingDetails fetches complaint details for all pending complaints.
//
// Uses goroutines for concurrent fetching (replaces the browser's Promise.allSettled).
// Each fetch is an authenticated HTTP GET via the session client.
//
// Parameters:
//   - sc: Authenticated session client
//   - stor: Storage containing pending complaint IDs and API IDs
//
// Returns:
//   - []Complaint: Complaint details for all pending complaints
//   - error: If no complaints found or all fetches fail
func FetchAllPendingDetails(sc *session.Client, stor *storage.Storage) ([]Complaint, error) {
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

	// Fetch all concurrently using goroutines
	complaints := fetchAllConcurrent(sc, requests)

	if len(complaints) == 0 {
		return nil, fmt.Errorf("failed to fetch any complaint details")
	}

	log.Printf("📊 Successfully fetched %d/%d complaint details", len(complaints), len(requests))
	return complaints, nil
}

// fetchAllConcurrent fetches all complaints concurrently using goroutines.
// A semaphore limits concurrency to 10 to avoid overwhelming the server.
func fetchAllConcurrent(sc *session.Client, requests []complaintRequest) []Complaint {
	type result struct {
		complaint *Complaint
		err       error
	}

	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)

	results := make([]result, len(requests))
	var wg sync.WaitGroup

	for i, r := range requests {
		wg.Add(1)
		go func(idx int, req complaintRequest) {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot
			c, err := fetchComplaintDetail(sc, req.apiID, req.complaintNo)
			results[idx] = result{complaint: c, err: err}
		}(i, r)
	}

	wg.Wait()

	var complaints []Complaint
	for _, r := range results {
		if r.err != nil {
			log.Printf("  ⚠️  Failed to fetch complaint detail: %v", r.err)
			continue
		}
		if r.complaint != nil {
			complaints = append(complaints, *r.complaint)
		}
	}
	return complaints
}

// fetchComplaintDetail fetches a single complaint's details from the API.
func fetchComplaintDetail(sc *session.Client, apiID, complaintNumber string) (*Complaint, error) {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", apiID)

	body, err := sc.GetJSON(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	var fullData map[string]interface{}
	if err := json.Unmarshal(body, &fullData); err != nil {
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
