// Package complaint handles complaint fetching and processing.
package complaint

import (
	"context"
	"log"

	"cmon/internal/config"
	"cmon/internal/errors"
	"cmon/internal/storage"
	"cmon/internal/telegram"

	"github.com/chromedp/chromedp"
)

// Fetcher orchestrates the complaint fetching process with concurrent processing.
//
// Architecture:
//   - Main thread: Navigates pages and scrapes complaint links
//   - Worker pool: Processes complaints concurrently
//   - Storage: Deduplicates and persists data
//   - Telegram: Sends notifications
//
// Flow:
//   1. Navigate to dashboard
//   2. For each page (up to MaxPages):
//      a. Scrape complaint links from table
//      b. Filter new complaints (not in storage)
//      c. Submit new complaints to worker pool
//      d. Workers fetch details and send to Telegram concurrently
//      e. Collect results and save to storage in batches
//      f. Navigate to next page
//   3. Return all active complaint IDs found
type Fetcher struct {
	ctx     context.Context
	storage *storage.Storage
	tg      *telegram.Client
	cfg     *config.Config
}

// New creates a new complaint fetcher.
//
// Parameters:
//   - ctx: Browser context for automation
//   - storage: Storage for deduplication and persistence
//   - tg: Telegram client for notifications
//   - cfg: Application configuration
//
// Returns:
//   - *Fetcher: Ready-to-use fetcher instance
func New(ctx context.Context, storage *storage.Storage, tg *telegram.Client, cfg *config.Config) *Fetcher {
	return &Fetcher{
		ctx:     ctx,
		storage: storage,
		tg:      tg,
		cfg:     cfg,
	}
}

// FetchAll fetches all complaints from the dashboard with pagination.
//
// This is the main entry point for complaint fetching.
//
// Concurrency model:
//   - Page navigation: Sequential (one page at a time)
//   - Complaint processing: Concurrent (worker pool)
//   - Storage writes: Batched for efficiency
//
// Error handling:
//   - Session expiry: Returns SessionExpiredError
//   - Navigation timeout: Returns FetchError
//   - Individual complaint failures: Logged but don't stop processing
//
// Parameters:
//   - baseURL: Dashboard URL to start fetching from
//
// Returns:
//   - []string: List of all active complaint IDs found
//   - error: Session expiry, navigation failure, or other critical errors
func (f *Fetcher) FetchAll(baseURL string) ([]string, error) {
	var allActiveComplaintIDs []string

	log.Println("  → Navigating to complaints dashboard...")

	// Step 1: Initial navigation with timeout
	navCtx, navCancel := context.WithTimeout(f.ctx, f.cfg.NavigationTimeout)
	err := chromedp.Run(navCtx, chromedp.Navigate(baseURL))
	navCancel()

	if err != nil {
		log.Printf("  ✗ Navigation timeout/error after %v: %v", f.cfg.NavigationTimeout, err)
		// Check if session expired (redirected to login)
		if isSessionExpired(f.ctx) {
			return nil, errors.NewSessionExpiredError("dashboard navigation failed")
		}
		return nil, errors.NewFetchError("failed to navigate to dashboard", err)
	}

	// Wait for table to be visible
	waitCtx, waitCancel := context.WithTimeout(f.ctx, f.cfg.WaitTimeout)
	err = chromedp.Run(waitCtx, chromedp.WaitVisible("#dataTable", chromedp.ByID))
	waitCancel()

	if err != nil {
		log.Printf("  ✗ Wait timeout/error after %v: %v", f.cfg.WaitTimeout, err)
		if isSessionExpired(f.ctx) {
			return nil, errors.NewSessionExpiredError("dashboard not visible")
		}
		return nil, errors.NewFetchError("failed to load dashboard table", err)
	}
	log.Println("  ✓ Dashboard loaded")

	// Step 2: Process pages with pagination
	currentPage := 1
	for {
		// Check page limit
		if currentPage > f.cfg.MaxPages {
			log.Printf("🛑 Reached maximum page limit (%d). Stopping.", f.cfg.MaxPages)
			break
		}

		log.Printf("📄 Processing Page %d...", currentPage)

		// Scrape current page
		pageIDs, err := f.scrapePage()
		if err != nil {
			log.Printf("  ⚠️  Error scraping page %d: %v", currentPage, err)
			break
		}

		allActiveComplaintIDs = append(allActiveComplaintIDs, pageIDs...)

		// Find next page URL
		nextURL, err := f.getNextPageURL()
		if err != nil {
			log.Printf("  ⚠️  Error finding next page URL: %v", err)
			break
		}

		if nextURL == "" {
			log.Println("✅ Reached last page (No valid 'Next' link found)")
			break
		}

		// Navigate to next page
		log.Printf("  → Navigating to next page...")
		navCtx, navCancel := context.WithTimeout(f.ctx, f.cfg.NavigationTimeout)
		err = chromedp.Run(navCtx, chromedp.Navigate(nextURL))
		navCancel()

		if err != nil {
			log.Printf("  ⚠️  Navigation timeout/error after %v: %v", f.cfg.NavigationTimeout, err)
			break
		}

		// Wait for table
		waitCtx, waitCancel := context.WithTimeout(f.ctx, f.cfg.WaitTimeout)
		err = chromedp.Run(waitCtx, chromedp.WaitVisible("#dataTable", chromedp.ByID))
		waitCancel()

		if err != nil {
			log.Printf("  ⚠️  Wait timeout/error after %v: %v", f.cfg.WaitTimeout, err)
			break
		}

		currentPage++
	}

	log.Printf("🎉 Total active complaints found across %d pages: %d", currentPage, len(allActiveComplaintIDs))
	return allActiveComplaintIDs, nil
}

// scrapePage extracts and processes complaints from the current page.
//
// Processing pipeline:
//   1. Extract complaint links from DOM
//   2. Filter new complaints (not in storage)
//   3. Create worker pool for concurrent processing
//   4. Submit complaints to workers
//   5. Collect results
//   6. Batch save to storage
//
// Returns:
//   - []string: List of complaint IDs found on this page
//   - error: Scraping or processing error
func (f *Fetcher) scrapePage() ([]string, error) {
	var complaintLinks []Link

	// Extract complaint links from table using JavaScript
	// This is faster than using ChromeDP's DOM queries
	err := chromedp.Run(f.ctx,
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

	// Collect all IDs for return value
	var allIDsOnPage []string
	var newComplaints []Link

	// Deduplicate and filter new complaints
	seenOnPage := make(map[string]bool)
	for _, complaint := range complaintLinks {
		allIDsOnPage = append(allIDsOnPage, complaint.ComplaintNumber)

		// Skip duplicates on same page
		if seenOnPage[complaint.ComplaintNumber] {
			continue
		}
		seenOnPage[complaint.ComplaintNumber] = true

		// Check if new (not in storage)
		if f.storage.IsNew(complaint.ComplaintNumber) {
			newComplaints = append(newComplaints, complaint)
			log.Println("    🆕 New Complaint -", complaint.ComplaintNumber)
		}
	}

	// Process new complaints concurrently if any found
	if len(newComplaints) > 0 {
		f.processComplaintsConcurrently(newComplaints)
	}

	return allIDsOnPage, nil
}

// processComplaintsConcurrently processes complaints using a worker pool.
//
// Concurrency flow:
//   1. Create worker pool with configured number of workers
//   2. Submit all complaints to job queue
//   3. Close job queue (signals no more jobs)
//   4. Collect results from workers
//   5. Batch save successful results to storage
//
// Parameters:
//   - complaints: List of new complaints to process
func (f *Fetcher) processComplaintsConcurrently(complaints []Link) {
	// Create map to lookup API ID by complaint number
	apiIDMap := make(map[string]string)
	for _, c := range complaints {
		apiIDMap[c.ComplaintNumber] = c.APIID
	}

	// Create worker pool
	pool := NewWorkerPool(f.ctx, f.tg, f.cfg.WorkerPoolSize)

	// Submit all jobs
	go func() {
		for _, complaint := range complaints {
			pool.Submit(complaint)
		}
		pool.Close() // Signal no more jobs
	}()

	// Collect results
	var recordsToSave []storage.Record
	for result := range pool.Results() {
		if result.Error != nil {
			log.Printf("    ⚠️  Failed to process %s: %v", result.ComplaintID, result.Error)
			continue
		}

		// Only save if we got a message ID
		if result.MessageID != "" {
			recordsToSave = append(recordsToSave, storage.Record{
				ComplaintID:  result.ComplaintID,
				MessageID:    result.MessageID,
				APIID:        apiIDMap[result.ComplaintID], // Get API ID from map
				ConsumerName: result.ConsumerName,
			})
		}
	}

	// Batch save to storage
	if len(recordsToSave) > 0 {
		if err := f.storage.SaveMultiple(recordsToSave); err != nil {
			log.Println("    ⚠️  Failed to save records:", err)
		} else {
			log.Printf("    ✓ Saved %d new complaints", len(recordsToSave))
		}
	}
}

// getNextPageURL finds the URL for the next page in pagination.
//
// Detection strategy:
//   1. Look for <a rel="next"> (standard pagination)
//   2. Look for pagination links with text "›", "Next", or "»"
//   3. Return empty string if no next page found
//
// Returns:
//   - string: URL of next page, or empty if last page
//   - error: JavaScript evaluation error
func (f *Fetcher) getNextPageURL() (string, error) {
	var nextURL string

	err := chromedp.Run(f.ctx,
		chromedp.Evaluate(`
			(function() {
				// Try standard rel="next" first
				const realNextLink = document.querySelector('a[rel="next"]');
				if (realNextLink && realNextLink.href) {
					return realNextLink.href;
				}
				
				// Fallback: Look for pagination links
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

// isSessionExpired checks if the current page indicates session expiration.
//
// This is a helper function that wraps the auth package's IsSessionExpired.
//
// Parameters:
//   - ctx: Browser context to check
//
// Returns:
//   - bool: true if session expired, false if still valid
func isSessionExpired(ctx context.Context) bool {
	var loginFormExists bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("#email_or_username") !== null`, &loginFormExists),
	)
	return err == nil && loginFormExists
}
