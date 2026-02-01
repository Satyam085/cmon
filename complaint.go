package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// ComplaintLink holds the complaint number and API ID
type ComplaintLink struct {
	ComplaintNumber string
	APIID           string
}

// FetchComplaints orchestrates the fetching of complaints.
func FetchComplaints(ctx context.Context, baseURL string, storage *ComplaintStorage, telegramConfig *TelegramConfig, maxPages int) ([]string, error) {
	var allActiveComplaintIDs []string

	log.Println("  → Navigating to complaints dashboard...")
	// 1. Initial Navigation
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitVisible("#dataTable", chromedp.ByID),
	); err != nil {
		if IsSessionExpired(ctx) {
			return nil, NewSessionExpiredError("dashboard not visible")
		}
		return nil, NewFetchError("failed to load dashboard", err)
	}
	log.Println("  ✓ Dashboard loaded")

	currentPage := 1
	for {
		if currentPage > maxPages {
			log.Printf("🛑 Reached maximum page limit (%d). Stopping.", maxPages)
			break
		}

		log.Printf("📄 Processing Page %d...", currentPage)

		// 2. Scrape the current view
		pageIDs, err := scrapeCurrentPage(ctx, storage, telegramConfig)
		if err != nil {
			log.Printf("  ⚠️  Error scraping page %d: %v", currentPage, err)
			break
		}

		allActiveComplaintIDs = append(allActiveComplaintIDs, pageIDs...)

		// 3. Find the URL for the next page
		nextURL, err := getNextPageURL(ctx)
		if err != nil {
			log.Printf("  ⚠️  Error finding next page URL: %v", err)
			break
		}

		if nextURL == "" {
			log.Println("✅ Reached last page (No valid 'Next' link found)")
			break
		}

		// 4. Navigate to the next page explicitly
		log.Printf("  → Navigating to next page...")
		err = chromedp.Run(ctx,
			chromedp.Navigate(nextURL),
			chromedp.WaitVisible("#dataTable", chromedp.ByID),
		)
		if err != nil {
			log.Printf("  ⚠️  Failed to navigate to next page: %v", err)
			break
		}

		currentPage++
	}

	log.Printf("🎉 Total active complaints found across %d pages: %d", currentPage, len(allActiveComplaintIDs))
	return allActiveComplaintIDs, nil
}

// scrapeCurrentPage extracts data from the currently visible table
func scrapeCurrentPage(ctx context.Context, storage *ComplaintStorage, telegramConfig *TelegramConfig) ([]string, error) {
	var complaintLinks []ComplaintLink

	// Extract data from the DOM
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll("#dataTable tbody tr")).map(row => {
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
		return nil, err
	}

	log.Println("    → Found", len(complaintLinks), "complaints on this page")

	var allIDsOnPage []string
	var newComplaintsToSave []ComplaintRecord

	for _, complaint := range complaintLinks {
		allIDsOnPage = append(allIDsOnPage, complaint.ComplaintNumber)

		if storage.IsNew(complaint.ComplaintNumber) {
			log.Println("    🆕 New Complaint -", complaint.ComplaintNumber)

			// Add delay to prevent rate limiting or race conditions
			time.Sleep(1 * time.Second)

			messageID := FetchComplaintDetails(ctx, complaint.APIID, complaint.ComplaintNumber, telegramConfig)

			storage.MarkAsSeen(complaint.ComplaintNumber)

			if messageID != "" {
				newComplaintsToSave = append(newComplaintsToSave, ComplaintRecord{
					ComplaintID: complaint.ComplaintNumber,
					MessageID:   messageID,
				})
			}
		}
	}

	if len(newComplaintsToSave) > 0 {
		if err := storage.SaveMultiple(newComplaintsToSave); err != nil {
			log.Println("    ⚠️  Failed to save records:", err)
		} else {
			for _, c := range newComplaintsToSave {
				storage.SetMessageID(c.ComplaintID, c.MessageID)
			}
		}
	}

	return allIDsOnPage, nil
}

// getNextPageURL checks for the next page link and returns the href URL.
func getNextPageURL(ctx context.Context) (string, error) {
	var nextURL string

	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const realNextLink = document.querySelector('a[rel="next"]');
				if (realNextLink && realNextLink.href) {
					return realNextLink.href;
				}
				const pageLinks = Array.from(document.querySelectorAll('ul.pagination li:not(.disabled) a.page-link'));
				for (let link of pageLinks) {
					const text = link.innerText.trim();
					if (text === '›' || text === 'Next' || text === '»') {
						return link.href;
					}
				}
				return "";
			})()
		`, &nextURL),
	)

	return nextURL, err
}

// FetchComplaintDetails executes a fetch() inside the browser to get details.
// This ensures cookies/session are reused correctly.
func FetchComplaintDetails(ctx context.Context, apiID string, complaintNumber string, telegramConfig *TelegramConfig) string {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", apiID)

	var jsonResponse string

	// MUST use async/await + AwaitPromise here.
	// A .then() chain returns a pending Promise object to chromedp, which then
	// tries to unmarshal that object directly into &jsonResponse (a string) and
	// fails with: "json: cannot unmarshal object into Go value of type string".
	// WithAwaitPromise(true) tells the CDP Runtime.evaluate to wait for the
	// Promise to settle and return the resolved string value instead.
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
		log.Printf("  ⚠️  Failed to fetch complaint details (Browser API): %v", err)
		return ""
	}

	if jsonResponse == "" {
		log.Println("  ⚠️  API returned empty response")
		return ""
	}

	var fullData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResponse), &fullData); err != nil {
		log.Println("  ⚠️  Failed to parse JSON:", err)
		return ""
	}

	// Extract complaintdetail
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
		log.Println("  ⚠️  complaintdetail missing in API response")
		return ""
	}

	prettyJSON, _ := json.MarshalIndent(structuredComplaint, "  ", "  ")

	// Send to Telegram
	if telegramConfig != nil {
		msgID, err := telegramConfig.SendComplaintMessage(string(prettyJSON), complaintNumber)
		if err == nil {
			return msgID
		}
		log.Println("⚠️  Failed to send Telegram notification:", err)
	}

	return ""
}