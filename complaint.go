package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/chromedp/chromedp"
)

// ComplaintLink holds the complaint number and API ID
type ComplaintLink struct {
	ComplaintNumber string
	APIID           string
}

// FetchComplaints orchestrates the fetching of complaints from all pages.
func FetchComplaints(ctx context.Context, baseURL string, storage *ComplaintStorage, telegramConfig *TelegramConfig) ([]ComplaintRecord, error) {
	var allNewComplaints []ComplaintRecord
	currentPage := 1

	for {
		pageURL := fmt.Sprintf("%s&page=%d", baseURL, currentPage)
		log.Printf("📄 Fetching complaints from page %d...", currentPage)

		newComplaints, hasNextPage, err := fetchComplaintsFromPage(ctx, pageURL, storage, telegramConfig)
		if err != nil {
			// If it's a session-expired error, we should stop and report it
			if _, ok := err.(*SessionExpiredError); ok {
				return allNewComplaints, err
			}
			// For other errors, we might want to log and continue, or stop
			log.Printf("  ⚠️  Error fetching page %d: %v", currentPage, err)
			break // Stop on any error for now
		}

		if len(newComplaints) > 0 {
			allNewComplaints = append(allNewComplaints, newComplaints...)
		}

		// Check if there are more pages by looking for the Next button
		if !hasNextPage {
			log.Println("✅ No more pages available")
			break
		}

		currentPage++
	}

	log.Printf("🎉 Total new complaints processed from all pages: %d", len(allNewComplaints))
	return allNewComplaints, nil
}

// fetchComplaintsFromPage fetches complaints from a single page URL.
// Returns: complaints, hasNextPage, error
func fetchComplaintsFromPage(ctx context.Context, url string, storage *ComplaintStorage, telegramConfig *TelegramConfig) ([]ComplaintRecord, bool, error) {
	log.Println("  → Navigating to complaints page...")

	var complaintLinks []ComplaintLink

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible("table", chromedp.ByQuery),
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll("table tbody tr")).map(row => {
				const link = row.querySelector('a[onclick*="openModelData"]');
				if (link) {
					const complaintNumber = link.innerText.trim();
					const onclickAttr = link.getAttribute('onclick');
					const match = onclickAttr.match(/openModelData\((\d+)\)/);
					const apiId = match ? match[1].toString() : '';
					return { ComplaintNumber: complaintNumber, APIID: apiId };
				}
				return null;
			}).filter(x => x !== null && x.APIID !== '')
		`, &complaintLinks),
	)
	if err != nil {
		log.Println("  ✗ Failed to fetch complaints:", err)
		if IsSessionExpired(ctx) {
			log.Println("  ⚠️  Session appears to be expired")
			return nil, false, NewSessionExpiredError("complaints table not visible")
		}
		return nil, false, NewFetchError("failed to navigate or extract complaints", err)
	}
	log.Println("  ✓ Complaints page loaded")

	log.Println("📄 Total complaints found on this page:", len(complaintLinks))

	var successfulComplaints []ComplaintRecord

	for _, complaint := range complaintLinks {
		if storage.IsNew(complaint.ComplaintNumber) {
			log.Println("🆕 New Complaint -", complaint.ComplaintNumber, "(API ID:", complaint.APIID, ")")
			messageID := FetchComplaintDetails(ctx, complaint.APIID, complaint.ComplaintNumber, telegramConfig)
			if messageID != "" {
				storage.MarkAsSeen(complaint.ComplaintNumber)
				successfulComplaints = append(successfulComplaints, ComplaintRecord{
					ComplaintID: complaint.ComplaintNumber,
					MessageID:   messageID,
				})
			}
		}
	}

	log.Println("🆕 New complaints processed on this page:", len(successfulComplaints))

	if len(successfulComplaints) > 0 {
		if err := storage.SaveMultiple(successfulComplaints); err != nil {
			log.Println("⚠️  Failed to save complaint records:", err)
		} else {
			log.Println("💾 Saved", len(successfulComplaints), "new complaint records")
			for _, c := range successfulComplaints {
				storage.SetMessageID(c.ComplaintID, c.MessageID)
			}
		}
	}

	// Check for next page using pagination controls
	var hasNextPage bool
	chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const nextBtn = document.querySelector('a[rel="next"]');
				if (!nextBtn) return false;
				// Check if next button is disabled
				const parentLi = nextBtn.closest('li');
				if (parentLi && parentLi.classList.contains('disabled')) return false;
				// Check if href exists and is valid
				const href = nextBtn.getAttribute('href');
				return href && href.trim() !== '' && !href.includes('page=') || href;
			})()
		`, &hasNextPage),
	)

	return successfulComplaints, hasNextPage, nil
}

// FetchComplaintDetails fetches the complaint details from the API
// Returns: messageID (empty string if Telegram not configured or failed)
func FetchComplaintDetails(ctx context.Context, apiID string, complaintNumber string, telegramConfig *TelegramConfig) string {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", apiID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Println("  ⚠️  Failed to create request:", err)
		return ""
	}

	// Add required headers
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	// Use the shared HTTP client
	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		log.Println("  ⚠️  Failed to fetch complaint:", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("  ⚠️  Failed to read response:", err)
		return ""
	}

	// Parse the full JSON response
	var fullData map[string]interface{}
	err = json.Unmarshal(body, &fullData)
	if err != nil {
		log.Println("  ⚠️  Failed to parse JSON:", err)
		return ""
	}

	// Extract complaintdetail and restructure
	type StructuredComplaint struct {
		ComplainNo      interface{} `json:"complain_no"`
		ConsumerNo      interface{} `json:"consumer_no"`
		ComplainantName interface{} `json:"complainant_name"`
		MobileNo        interface{} `json:"mobile_no"`
		Description     interface{} `json:"description"`
		ComplainDate    interface{} `json:"complain_date"`
		ExactLocation   interface{} `json:"exact_location"`
		Area            interface{} `json:"area"`
	}

	var structuredComplaint StructuredComplaint

	if complaintDetail, ok := fullData["complaintdetail"].(map[string]interface{}); ok {
		structuredComplaint = StructuredComplaint{
			ComplainNo:      complaintDetail["complain_no"],
			ConsumerNo:      complaintDetail["consumer_no"],
			ComplainantName: complaintDetail["complainant_name"],
			MobileNo:        complaintDetail["mobile_no"],
			Description:     complaintDetail["description"],
			ComplainDate:    complaintDetail["complain_date"],
			ExactLocation:   complaintDetail["exact_location"],
			Area:            complaintDetail["area"],
		}
	} else {
		log.Println("  ⚠️  complaintdetail not found in response")
		return ""
	}

	prettyJSON, err := json.MarshalIndent(structuredComplaint, "  ", "  ")
	if err != nil {
		log.Println("  ⚠️  Failed to format JSON:", err)
		return ""
	}

	fmt.Println("\n════════════════════════════════════════════════════════")
	fmt.Printf("Complaint Number: %s (API ID: %s)\n", complaintNumber, apiID)
	fmt.Println("────────────────────────────────────────────────────────")
	fmt.Println(string(prettyJSON))
	fmt.Println("════════════════════════════════════════════════════════")

	// Send to Telegram if configured
	var messageID string
	if telegramConfig != nil {
		msgID, err := telegramConfig.SendComplaintMessage(string(prettyJSON), complaintNumber)
		if err != nil {
			log.Println("⚠️  Failed to send Telegram notification:", err)
		} else {
			messageID = msgID
		}
	}

	return messageID
}
