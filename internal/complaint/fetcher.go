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
			return nil, errors.NewFetchError(fmt.Sprintf("failed to scrape page %d", currentPage), err)
		}
		allActiveComplaintIDs = append(allActiveComplaintIDs, pageIDs...)

		// Find next page URL from current document
		nextURL := getNextPageURL(doc)
		if nextURL == "" {
			break
		}

		doc, err = f.sc.GetDoc(nextURL)
		if err != nil {
			return nil, errors.NewFetchError(fmt.Sprintf("failed to fetch page %d", currentPage+1), err)
		}

		// Session check on each new page
		if doc.Find("#email_or_username").Length() > 0 {
			return nil, errors.NewSessionExpiredError("session expired during pagination")
		}
		if doc.Find("#dataTable").Length() == 0 {
			return nil, errors.NewFetchError(fmt.Sprintf("page %d loaded but #dataTable not found", currentPage+1), nil)
		}

		currentPage++
	}

	return allActiveComplaintIDs, nil
}

// scrapePage extracts links from the current page and processes new complaints.
func (f *Fetcher) scrapePage(doc *goquery.Document) ([]string, error) {
	if doc.Find("#dataTable").Length() == 0 {
		return nil, fmt.Errorf("#dataTable not found")
	}

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
		if err := f.processComplaintsConcurrently(newComplaints); err != nil {
			return nil, err
		}
	}

	return allIDsOnPage, nil
}

// processComplaintsConcurrently processes complaints using a worker pool.
func (f *Fetcher) processComplaintsConcurrently(complaints []Link) error {
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
		if len(complaints) > 0 {
			return fmt.Errorf("failed to process any of %d new complaints", len(complaints))
		}
		return nil
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

	type notification struct {
		ComplaintID   string
		ComplaintJSON string
		GujaratiText  string
		WAText        string
	}

	// Phase 3: Persist complaint records before any external side effects.
	var recordsToSave []storage.Record
	var notifications []notification
	for i, res := range results {
		gujoName := translations[i].name
		gujoDesc := translations[i].desc
		gujoAddr := translations[i].addr

		gujaratiText := ""
		if gujoName != "" || gujoDesc != "" || gujoAddr != "" {
			gujaratiText = fmt.Sprintf("👤 %s\n💬 %s\n📍 %s", gujoName, gujoDesc, gujoAddr)
		}

		prettyJSON, _ := json.MarshalIndent(res.Details, "  ", "  ")

		record := storage.Record{
			ComplaintID:  res.ComplaintID,
			APIID:        apiIDMap[res.ComplaintID],
			ConsumerName: res.ConsumerName,
			Village:      res.Details.Village,
			Belt:         res.Details.Belt,
		}
		recordsToSave = append(recordsToSave, record)
		notifications = append(notifications, notification{
			ComplaintID:   res.ComplaintID,
			ComplaintJSON: string(prettyJSON),
			GujaratiText:  gujaratiText,
			WAText:        buildWhatsAppMessage(res.Details, gujaratiText),
		})
	}

	// Complaint identity and metadata must be durable before we emit channel
	// notifications, otherwise the DB can fall behind visible side effects.
	if len(recordsToSave) > 0 {
		if err := f.storage.SaveMultiple(recordsToSave); err != nil {
			return fmt.Errorf("failed to save complaint records: %w", err)
		}
	}

	// Phase 4: Telegram notifications + message ID persistence
	if f.tg != nil {
		for _, n := range notifications {
			msgID, err := f.tg.SendComplaintMessage(n.ComplaintJSON, n.ComplaintID, n.GujaratiText)
			if err != nil {
				log.Printf("    ⚠️  Failed to send Telegram msg for %s: %v", n.ComplaintID, err)
				continue
			}
			if msgID == "" {
				log.Printf("    ⚠️  Telegram sent complaint %s but returned no message ID", n.ComplaintID)
				continue
			}
			if err := f.storage.SetMessageID(n.ComplaintID, msgID); err != nil {
				log.Printf("    ⚠️  Failed to persist Telegram message ID for %s: %v", n.ComplaintID, err)
			}
		}
	}

	// Phase 5: WhatsApp Notifications
	// Sends are intentionally sequential with a 1s gap between each one.
	// whatsmeow prefetches encryption sessions from its internal SQLite DB when
	// sending — firing multiple sends too quickly causes SQLITE_BUSY contention.
	if f.wa != nil {
		for i, n := range notifications {
			if i > 0 {
				time.Sleep(1 * time.Second)
			}

			if err := f.wa.SendComplaintMessage(n.WAText, n.ComplaintID, f.storage); err != nil {
				log.Printf("    ⚠️  Failed to send WhatsApp msg for %s: %v", n.ComplaintID, err)
			}
		}
	}

	return nil
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
			"%s Belt: %s\n"+
			"👤 %s\n"+
			"📞 %s\n"+
			"🆔 Consumer: %s\n"+
			"📅 %s\n\n"+
			"💬 Details:\n%s\n"+
			"📍 %s, %s",
		str(details.ComplainNo),
		belt.StyleFor(details.Belt).Emoji,
		belt.DisplayName(details.Belt),
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

func displayBelt(name string) string {
	return belt.MessageLabel(name)
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
