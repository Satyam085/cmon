// Package complaint handles complaint fetching and processing.
package complaint

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"cmon/internal/belt"
	"cmon/internal/config"
	"cmon/internal/errors"
	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/telegram"
	"cmon/internal/translate"
	"cmon/internal/whatsapp"

	"github.com/PuerkitoBio/goquery"
)

// Fetcher orchestrates the complaint fetching process with concurrent processing.
//
// Architecture:
//   - Main thread: Navigates pages and scrapes complaint links via HTTP + goquery
//   - Worker pool: Processes complaints concurrently via HTTP API calls
//   - Storage: Deduplicates and persists data
//   - Telegram: Sends notifications
type Fetcher struct {
	sc         *session.Client
	storage    *storage.Storage
	tg         *telegram.Client
	wa         *whatsapp.Client
	cfg        *config.Config
	translator *translate.Translator
}

// New creates a new complaint fetcher.
func New(sc *session.Client, storage *storage.Storage, tg *telegram.Client, wa *whatsapp.Client, cfg *config.Config, translator *translate.Translator) *Fetcher {
	return &Fetcher{
		sc:         sc,
		storage:    storage,
		tg:         tg,
		wa:         wa,
		cfg:        cfg,
		translator: translator,
	}
}

// FetchAll fetches all complaints from the dashboard with pagination.
//
// Parameters:
//   - baseURL: Dashboard URL to start fetching from
//
// Returns:
//   - []string: List of all active complaint IDs found
//   - error: Session expiry, navigation failure, or other critical errors
func (f *Fetcher) FetchAll(baseURL string) ([]string, error) {
	var allActiveComplaintIDs []string

	// Fetch first page
	doc, err := f.sc.GetDoc(baseURL)
	if err != nil {
		return nil, errors.NewFetchError("failed to navigate to dashboard", err)
	}

	// Session expiry check: login form present in returned HTML
	if doc.Find("#email_or_username").Length() > 0 {
		return nil, errors.NewSessionExpiredError("dashboard navigation showed login form")
	}

	// Verify table is present
	if doc.Find("#dataTable").Length() == 0 {
		return nil, errors.NewFetchError("dashboard loaded but #dataTable not found", nil)
	}

	currentPage := 1
	for {
		if currentPage > f.cfg.MaxPages {
			log.Printf("🛑 Reached maximum page limit (%d). Stopping.", f.cfg.MaxPages)
			break
		}

		pageIDs, err := f.scrapePage(doc)
		if err != nil {
			log.Printf("  ⚠️  Error scraping page %d: %v", currentPage, err)
			break
		}
		allActiveComplaintIDs = append(allActiveComplaintIDs, pageIDs...)

		// Find next page URL from current document
		nextURL := getNextPageURL(doc)
		if nextURL == "" {
			break
		}

		doc, err = f.sc.GetDoc(nextURL)
		if err != nil {
			break
		}

		// Session check on each new page
		if doc.Find("#email_or_username").Length() > 0 {
			return nil, errors.NewSessionExpiredError("session expired during pagination")
		}

		currentPage++
	}

	return allActiveComplaintIDs, nil
}

// scrapePage extracts links from the current page and processes new complaints.
func (f *Fetcher) scrapePage(doc *goquery.Document) ([]string, error) {
	complaintLinks := extractLinks(doc)

	var allIDsOnPage []string
	var newComplaints []Link

	seenOnPage := make(map[string]bool)
	for _, complaint := range complaintLinks {
		allIDsOnPage = append(allIDsOnPage, complaint.ComplaintNumber)

		if seenOnPage[complaint.ComplaintNumber] {
			continue
		}
		seenOnPage[complaint.ComplaintNumber] = true

		if f.storage.IsNew(complaint.ComplaintNumber) {
			newComplaints = append(newComplaints, complaint)
		}
	}

	if len(newComplaints) > 0 {
		f.processComplaintsConcurrently(newComplaints)
	}

	return allIDsOnPage, nil
}

// processComplaintsConcurrently processes complaints using a worker pool.
func (f *Fetcher) processComplaintsConcurrently(complaints []Link) {
	apiIDMap := make(map[string]string)
	for _, c := range complaints {
		apiIDMap[c.ComplaintNumber] = c.APIID
	}

	pool := NewWorkerPool(f.sc, f.cfg.WorkerPoolSize, len(complaints))

	go func() {
		for _, complaint := range complaints {
			pool.Submit(complaint)
		}
		pool.Close()
	}()

	var results []ProcessResult
	for result := range pool.Results() {
		if result.Error != nil {
			continue
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		return
	}

	safeStr := func(v interface{}) string {
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}

	// Phase 2: Translate each complaint individually.
	// BatchTranslateToGujarati takes exactly 3 texts [name, desc, addr] for ONE complaint.
	type translationResult struct {
		name, desc, addr string
	}
	translations := make([]translationResult, len(results))

	for i, res := range results {
		match := belt.Resolve(
			safeStr(res.Details.Area),
			safeStr(res.Details.ExactLocation),
			safeStr(res.Details.Description),
		)
		res.Details.Village = match.Village
		res.Details.Belt = match.Belt
		results[i].Details = res.Details

		name := safeStr(res.Details.ComplainantName)
		desc := safeStr(res.Details.Description)
		loc := safeStr(res.Details.ExactLocation)
		area := safeStr(res.Details.Area)
		addr := fmt.Sprintf("%s, %s", loc, area)

		if f.translator != nil {
			texts := []string{name, desc, addr}
			out, err := f.translator.BatchTranslateToGujarati(context.Background(), texts)
			if err != nil {
				translations[i] = translationResult{name, desc, addr}
			} else {
				translations[i] = translationResult{out[0], out[1], out[2]}
			}
		} else {
			translations[i] = translationResult{name, desc, addr}
		}
	}

	// Phase 3 & 4: Telegram Notifications and DB Save
	var recordsToSave []storage.Record
	for i, res := range results {
		gujoName := translations[i].name
		gujoDesc := translations[i].desc
		gujoAddr := translations[i].addr

		gujaratiText := ""
		if gujoName != "" || gujoDesc != "" || gujoAddr != "" {
			gujaratiText = fmt.Sprintf("👤 %s\n💬 %s\n📍 %s", gujoName, gujoDesc, gujoAddr)
		}

		prettyJSON, _ := json.MarshalIndent(res.Details, "  ", "  ")

		var messageID string
		if f.tg != nil {
			msgID, err := f.tg.SendComplaintMessage(string(prettyJSON), res.ComplaintID, gujaratiText)
			if err != nil {
				log.Printf("    ⚠️  Failed to send Telegram msg for %s: %v", res.ComplaintID, err)
			} else {
				messageID = msgID
			}
		}

		record := storage.Record{
			ComplaintID:  res.ComplaintID,
			MessageID:    messageID,
			APIID:        apiIDMap[res.ComplaintID],
			ConsumerName: res.ConsumerName,
			Village:      res.Details.Village,
			Belt:         res.Details.Belt,
		}
		recordsToSave = append(recordsToSave, record)
	}

	// Batch save to storage FIRST
	if len(recordsToSave) > 0 {
		if err := f.storage.SaveMultiple(recordsToSave); err != nil {
			log.Println("    ⚠️  Failed to save records:", err)
		}
	}

	// Phase 5: WhatsApp Notifications
	// Sends are intentionally sequential with a 1s gap between each one.
	// whatsmeow prefetches encryption sessions from its internal SQLite DB when
	// sending — firing multiple sends too quickly causes SQLITE_BUSY contention.
	if f.wa != nil {
		for i, res := range results {
			if i > 0 {
				time.Sleep(1 * time.Second)
			}

			gujoName := translations[i].name
			gujoDesc := translations[i].desc
			gujoAddr := translations[i].addr

			gujaratiText := ""
			if gujoName != "" || gujoDesc != "" || gujoAddr != "" {
				gujaratiText = fmt.Sprintf("👤 %s\n💬 %s\n📍 %s", gujoName, gujoDesc, gujoAddr)
			}

			waText := buildWhatsAppMessage(res.Details, gujaratiText)
			if err := f.wa.SendComplaintMessage(waText, res.ComplaintID, f.storage); err != nil {
				log.Printf("    ⚠️  Failed to send WhatsApp msg for %s: %v", res.ComplaintID, err)
			}
		}
	}

}

// buildWhatsAppMessage formats complaint details as plain text for WhatsApp.
func buildWhatsAppMessage(details Details, gujaratiText string) string {
	str := func(v interface{}) string {
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}

	msg := fmt.Sprintf(
		"📋 Complaint: %s\n\n"+
			"🏷️ Belt: %s\n"+
			"👤 %s\n"+
			"📞 %s\n"+
			"🆔 Consumer: %s\n"+
			"📅 %s\n\n"+
			"💬 Details:\n%s\n"+
			"📍 %s, %s",
		str(details.ComplainNo),
		displayBelt(details.Belt),
		str(details.ComplainantName),
		str(details.MobileNo),
		str(details.ConsumerNo),
		str(details.ComplainDate),
		str(details.Description),
		str(details.ExactLocation),
		str(details.Area),
	)

	if gujaratiText != "" {
		msg += "\n\n" + strings.Repeat("─", 10) + "\n" + gujaratiText
	}

	return msg
}

func displayBelt(belt string) string {
	if strings.TrimSpace(belt) == "" {
		return "Unknown"
	}
	return belt
}

// onclickRe matches the API ID from onclick="openModelData(12345)"
var onclickRe = regexp.MustCompile(`openModelData\((\d+)\)`)

// extractLinks extracts complaint number + API ID pairs from the #dataTable rows.
func extractLinks(doc *goquery.Document) []Link {
	var links []Link
	doc.Find("#dataTable tbody tr").Each(func(_ int, row *goquery.Selection) {
		anchor := row.Find(`a[onclick*="openModelData"]`)
		if anchor.Length() == 0 {
			return
		}
		complaintNumber := strings.TrimSpace(anchor.Text())
		onclick := anchor.AttrOr("onclick", "")
		m := onclickRe.FindStringSubmatch(onclick)
		if len(m) < 2 || complaintNumber == "" {
			return
		}
		links = append(links, Link{
			ComplaintNumber: complaintNumber,
			APIID:           m[1],
		})
	})
	return links
}

// getNextPageURL finds the URL for the next page in pagination.
//
// Detection strategy:
//  1. Look for <a rel="next">
//  2. Look for pagination links with text "›", "Next", or "»"
//  3. Return empty string if no next page found
func getNextPageURL(doc *goquery.Document) string {
	// Strategy 1: standard rel="next"
	if href, exists := doc.Find(`a[rel="next"]`).Attr("href"); exists && href != "" {
		return href
	}

	// Strategy 2: pagination link by visible text
	var nextURL string
	doc.Find("ul.pagination li:not(.disabled) a.page-link").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		text := strings.TrimSpace(a.Text())
		if text == "›" || text == "Next" || text == "»" {
			if href, ok := a.Attr("href"); ok && href != "" {
				nextURL = href
				return false
			}
		}
		return true
	})
	return nextURL
}
