package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/chromedp/chromedp"
)

func FetchComplaints(ctx context.Context, url string, storage *ComplaintStorage) ([]string, error) {
	log.Println("  → Navigating to complaints page...")
	
	// Extract both complaint links with their onclick IDs and basic data
	type ComplaintLink struct {
		ComplaintNumber string
		APIID           string
	}
	
	var complaintLinks []ComplaintLink

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),

		// wait for table to load
		chromedp.WaitVisible("table", chromedp.ByQuery),

		// extract complaint numbers and API IDs from onclick attributes
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
		return nil, err
	}
	log.Println("  ✓ Complaints page loaded")

	log.Println("📄 Total complaints found:", len(complaintLinks))

	var successfulComplaintIDs []string
	
	// Process each complaint
	for _, complaint := range complaintLinks {
		if storage.IsNew(complaint.ComplaintNumber) {
			// Fetch and log complaint details from API
			log.Println("🆕 New Complaint -", complaint.ComplaintNumber, "(API ID:", complaint.APIID, ")")
			err := FetchComplaintDetails(ctx, complaint.APIID, complaint.ComplaintNumber)
			if err != nil {
				log.Println("  ⚠️  Error fetching details:", err)
			} else {
				// Only add to successful list if details were fetched successfully
				successfulComplaintIDs = append(successfulComplaintIDs, complaint.ComplaintNumber)
			}
		}
	}

	log.Println("🆕 New complaints processed:", len(successfulComplaintIDs))

	// Save only successfully processed complaint IDs to file
	if len(successfulComplaintIDs) > 0 {
		if err := storage.SaveMultiple(successfulComplaintIDs); err != nil {
			log.Println("⚠️  Failed to save complaint IDs:", err)
		} else {
			log.Println("💾 Saved", len(successfulComplaintIDs), "new complaint IDs")
		}
	}

	return successfulComplaintIDs, nil
}

// FetchComplaintDetails fetches the complaint details from the API
func FetchComplaintDetails(ctx context.Context, apiID string, complaintNumber string) error {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", apiID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add required headers
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	// Skip certificate verification for HTTPS
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch complaint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the full JSON response
	var fullData map[string]interface{}
	err = json.Unmarshal(body, &fullData)
	if err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
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
		return fmt.Errorf("complaintdetail not found in response")
	}

	prettyJSON, err := json.MarshalIndent(structuredComplaint, "  ", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	fmt.Println("\n════════════════════════════════════════════════════════")
	fmt.Printf("Complaint Number: %s (API ID: %s)\n", complaintNumber, apiID)
	fmt.Println("────────────────────────────────────────────────────────")
	fmt.Println(string(prettyJSON))
	fmt.Println("════════════════════════════════════════════════════════\n")

	return nil
}