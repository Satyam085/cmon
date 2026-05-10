// Package summary handles generating summary images for pending complaints.
package summary

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"cmon/internal/session"
	"cmon/internal/storage"
)

// pendingComplaint pairs a stored complaint ID with its API ID so the
// backfill path can fetch and persist details in one go.
type pendingComplaint struct {
	complaintID string
	apiID       string
}

// FetchAllPendingDetails returns one Complaint per active complaint in storage.
//
// Storage is the source of truth: scrape cycles populate every detail field
// when a complaint is first seen, so the dashboard does not need to call the
// DGVCL detail API on every refresh. Only rows that pre-date this caching
// behaviour (or whose details were missed) trigger a one-shot API backfill
// here, after which they too become storage-resident.
//
// Parameters:
//   - sc: Authenticated session client (used only for the rare backfill path)
//   - stor: Storage containing every pending complaint and its cached details
//
// Returns:
//   - []Complaint: One entry per complaint in storage that has an API ID
//   - error: Only when storage has no pending complaints
func FetchAllPendingDetails(sc *session.Client, stor *storage.Storage) ([]Complaint, error) {
	complaintIDs := stor.GetAllSeenComplaints()
	if len(complaintIDs) == 0 {
		return nil, fmt.Errorf("no pending complaints found")
	}

	complaints := make([]Complaint, 0, len(complaintIDs))
	var needsBackfill []pendingComplaint

	for _, id := range complaintIDs {
		apiID := stor.GetAPIID(id)
		if apiID == "" {
			log.Printf("  ⚠️  No API ID for complaint %s, skipping", id)
			continue
		}

		c := buildFromStorage(stor, id, apiID)
		if needsRefetch(c) {
			needsBackfill = append(needsBackfill, pendingComplaint{id, apiID})
			continue
		}
		complaints = append(complaints, c)
	}

	if len(needsBackfill) > 0 {
		log.Printf("📊 Backfilling details for %d legacy complaints (one-time per complaint)", len(needsBackfill))
		filled := backfillDetails(sc, stor, needsBackfill)
		complaints = append(complaints, filled...)
	}

	if len(complaints) == 0 {
		return nil, fmt.Errorf("no complaints with valid API IDs")
	}

	log.Printf("📊 Returning %d complaint records (storage-backed)", len(complaints))
	return complaints, nil
}

// buildFromStorage assembles a Complaint entirely from cached storage values.
func buildFromStorage(stor *storage.Storage, complaintID, apiID string) Complaint {
	date := stor.GetComplainDate(complaintID)
	return Complaint{
		ComplainNo:        complaintID,
		Name:              stor.GetConsumerName(complaintID),
		MobileNo:          stor.GetMobileNo(complaintID),
		Address:           stor.GetAddress(complaintID),
		Area:              stor.GetArea(complaintID),
		Village:           stor.GetVillage(complaintID),
		Belt:              stor.GetBelt(complaintID),
		Description:       stor.GetDescription(complaintID),
		ComplainDate:      date,
		TelegramMessageID: stor.GetMessageID(complaintID),
		WhatsAppMessageID: stor.GetWAMessageID(complaintID),
		APIID:             apiID,
		AgeMinutes:        computeAgeMinutes(date, time.Now()),
	}
}

// needsRefetch returns true when a Complaint built from storage is missing the
// detail fields that scrape would normally cache. Used to decide which rows to
// backfill from the API. ConsumerName alone is not enough — it has been stored
// since before this change — so we key off ComplainDate, the cheapest field
// to detect as "never cached".
func needsRefetch(c Complaint) bool {
	return c.ComplainDate == "" && c.Description == "" && c.MobileNo == ""
}

// backfillDetails fetches missing detail fields for legacy complaints and
// persists them so that subsequent dashboard loads stay storage-resident.
//
// The session.Client global rate limiter paces these calls so a one-time
// backfill of N legacy rows behaves like any other paced burst.
func backfillDetails(sc *session.Client, stor *storage.Storage, pending []pendingComplaint) []Complaint {
	type result struct {
		c  *Complaint
		ok bool
	}
	results := make([]result, len(pending))

	var wg sync.WaitGroup
	for i, p := range pending {
		wg.Add(1)
		go func(idx int, complaintID, apiID string) {
			defer wg.Done()
			c, err := fetchAndPersistDetail(sc, stor, complaintID, apiID)
			if err != nil {
				log.Printf("  ⚠️  Backfill failed for %s: %v", complaintID, err)
				return
			}
			results[idx] = result{c: c, ok: true}
		}(i, p.complaintID, p.apiID)
	}
	wg.Wait()

	out := make([]Complaint, 0, len(results))
	for _, r := range results {
		if r.ok && r.c != nil {
			out = append(out, *r.c)
		}
	}
	return out
}

// fetchAndPersistDetail hits the DGVCL detail API for a single complaint,
// writes the result into storage so future reads bypass the API, and returns
// the populated Complaint for immediate dashboard rendering.
func fetchAndPersistDetail(sc *session.Client, stor *storage.Storage, complaintID, apiID string) (*Complaint, error) {
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

	mobile := safeStr(detail["mobile_no"])
	address := safeStr(detail["exact_location"])
	area := safeStr(detail["area"])
	desc := safeStr(detail["description"])
	date := safeStr(detail["complain_date"])

	if err := stor.SetDetails(complaintID, mobile, address, area, desc, date); err != nil {
		// Persistence failure shouldn't fail the dashboard render — log and
		// continue with the in-memory Complaint we just built.
		log.Printf("  ⚠️  Failed to persist backfilled details for %s: %v", complaintID, err)
	}

	return &Complaint{
		ComplainNo:        safeStr(detail["complain_no"]),
		Name:              safeStr(detail["complainant_name"]),
		ConsumerNo:        safeStr(detail["consumer_no"]),
		MobileNo:          mobile,
		Address:           address,
		Area:              area,
		Village:           stor.GetVillage(complaintID),
		Belt:              stor.GetBelt(complaintID),
		Description:       desc,
		ComplainDate:      date,
		TelegramMessageID: stor.GetMessageID(complaintID),
		WhatsAppMessageID: stor.GetWAMessageID(complaintID),
		APIID:             apiID,
		AgeMinutes:        computeAgeMinutes(date, time.Now()),
	}, nil
}

// safeStr converts an interface{} value to string, handling nil.
func safeStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
