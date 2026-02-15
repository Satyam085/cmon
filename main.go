// CMON - Complaint Monitoring System
//
// This application monitors the DGVCL complaint portal and sends
// real-time notifications via Telegram when new complaints are filed.
//
// Architecture:
//   - Main thread: Orchestrates fetch loop and error recovery
//   - Health check server: Background HTTP server for monitoring
//   - Telegram handler: Background goroutine for processing callbacks
//   - Worker pool: Concurrent complaint processing (created per fetch)
//
// Flow:
//  1. Load configuration and initialize components
//  2. Login to DGVCL portal
//  3. Initial fetch of complaints
//  4. Start periodic refresh loop (every 15 minutes by default)
//  5. Handle errors with retry logic and browser restart
//  6. Graceful shutdown on SIGTERM/SIGINT
//
// Error recovery strategy:
//   - Session expired → Re-login
//   - Re-login failed → Restart browser and re-login
//   - All retries failed → Send critical alert to Telegram
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"cmon/internal/auth"
	"cmon/internal/browser"
	"cmon/internal/complaint"
	"cmon/internal/config"
	"cmon/internal/errors"
	"cmon/internal/health"
	"cmon/internal/storage"
	"cmon/internal/telegram"
	"cmon/internal/translate"
)

func main() {
	// Force Indian Standard Time (IST) for all time operations
	// This ensures consistent timestamps regardless of server timezone
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist

	// Application startup
	log.Println("🚀 Starting CMON application...")

	// Step 1: Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}
	log.Printf("✓ Loaded credentials for user: %s", cfg.Username)
	log.Printf("✓ Pagination limit: %d pages", cfg.MaxPages)
	log.Printf("✓ Worker pool size: %d workers", cfg.WorkerPoolSize)

	// Step 2: Initialize storage (loads existing complaints from CSV)
	log.Println("📋 Initializing complaint storage...")
	stor := storage.New()

	// Step 3: Initialize Telegram client (optional)
	log.Println("📱 Initializing Telegram...")
	tg := telegram.NewClient()

	// Step 3b: Initialize Google Cloud Translator (optional)
	log.Println("🌐 Initializing Gujarati translator...")
	translator, err := translate.NewTranslator(context.Background(), cfg.GoogleProjectID)
	if err != nil {
		log.Printf("⚠️  Translator init failed (translation disabled): %v", err)
	}
	if translator != nil {
		defer translator.Close()
	}

	// Step 4: Initialize health monitor
	healthMonitor := health.NewMonitor()

	// Step 5: Start health check server in background
	health.StartServer(healthMonitor, cfg.HealthCheckPort)

	// Step 6: Initialize browser context
	log.Println("🌐 Initializing browser context...")
	ctxHolder := browser.NewContextHolder()
	defer ctxHolder.Cancel()
	log.Println("✓ Browser context created")

	// Step 7: Start Telegram callback handler if configured
	if tg != nil {
		callbackCtx, callbackCancel := context.WithCancel(context.Background())
		defer callbackCancel()

		go tg.HandleUpdates(callbackCtx, ctxHolder, stor)
		log.Println("✓ Telegram callback handler started")
	}

	// Step 8: Login with retry logic
	log.Println("🔐 Attempting to login...")
	var loginErr error
	for attempt := 1; attempt <= cfg.MaxLoginRetries; attempt++ {
		log.Printf("   Login attempt %d/%d...", attempt, cfg.MaxLoginRetries)
		loginErr = auth.Login(ctxHolder.Get(), cfg.LoginURL, cfg.Username, cfg.Password)
		if loginErr == nil {
			log.Println("✓ Login successful")
			break
		}

		if attempt < cfg.MaxLoginRetries {
			log.Printf("   ❌ Login failed: %v", loginErr)
			log.Printf("   ⏳ Retrying in %v...", cfg.LoginRetryDelay)
			time.Sleep(cfg.LoginRetryDelay)
		}
	}

	if loginErr != nil {
		log.Fatal("❌ Login failed after", cfg.MaxLoginRetries, "attempts:", loginErr)
	}

	log.Println("⏳ Waiting for page to load...")
	time.Sleep(2 * time.Second)

	// Step 9: Initial fetch of complaints
	log.Println("📬 Fetching complaints...")
	fetcher := complaint.New(ctxHolder.Get(), stor, tg, cfg, translator)
	activeComplaintIDs, err := fetcher.FetchAll(cfg.ComplaintURL)
	if err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}

	// Step 10: Check for resolved complaints
	// (complaints in storage but not on website anymore)
	markResolvedComplaints(ctxHolder.Get(), stor, tg, activeComplaintIDs)

	// Update health monitor
	healthMonitor.UpdateFetchStatus("success")

	log.Println("✅ Initial fetch completed!")
	log.Printf("⏰ Starting refresh loop - will check every %v...\n", cfg.FetchInterval)
	log.Println("═══════════════════════════════════════════════════════════")

	// Step 11: Set up graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Step 12: Start refresh ticker
	ticker := time.NewTicker(cfg.FetchInterval)
	defer ticker.Stop()

	// Step 13: Main refresh loop
	for {
		select {
		case <-shutdownCtx.Done():
			log.Println("\n🛑 Shutdown signal received, cleaning up...")
			ctxHolder.Cancel()
			log.Println("✅ Cleanup complete, shutting down")
			return

		case <-ticker.C:
			log.Println("\n📬 Refreshing complaints list...")
			log.Println("⏰ Time:", time.Now().Format("2006-01-02 15:04:05"))

			// Attempt to fetch with full retry logic
			fetchErr := fetchWithRetry(
				ctxHolder,
				cfg.ComplaintURL,
				stor,
				tg,
				cfg.LoginURL,
				cfg.Username,
				cfg.Password,
				cfg,
				cfg.MaxFetchRetries,
				cfg.FetchTimeout,
				healthMonitor,
				translator,
			)

			if fetchErr != nil {
				log.Println("⚠️  Final error after all retry attempts:", fetchErr)
			}

			log.Println("═══════════════════════════════════════════════════════════")
		}
	}
}

// fetchWithRetry implements the complete error handling flow with retries.
//
// Retry strategy:
//   1. Attempt fetch with timeout
//   2. If session expired → Re-login and retry
//   3. If re-login failed → Restart browser, re-login, and retry
//   4. Repeat up to maxRetries times
//   5. If all retries failed → Send critical alert
//
// Error types handled:
//   - SessionExpiredError: Triggers re-login
//   - LoginFailedError: Triggers browser restart
//   - FetchError: Generic retry with delay
//   - Context timeout: Treated as fetch error
//
// Parameters:
//   - ctxHolder: Browser context holder (can be updated during restart)
//   - complaintURL: Dashboard URL to fetch from
//   - stor: Storage for deduplication
//   - tg: Telegram client for notifications
//   - loginURL: Login page URL
//   - username: DGVCL username
//   - password: DGVCL password
//   - cfg: Application configuration
//   - maxRetries: Maximum retry attempts
//   - fetchTimeout: Timeout for each fetch attempt
//   - healthMonitor: Health monitor to update status
//
// Returns:
//   - error: Final error if all retries failed, nil on success
func fetchWithRetry(
	ctxHolder *browser.ContextHolder,
	complaintURL string,
	stor *storage.Storage,
	tg *telegram.Client,
	loginURL, username, password string,
	cfg *config.Config,
	maxRetries int,
	fetchTimeout time.Duration,
	healthMonitor *health.Monitor,
	translator *translate.Translator,
) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔄 Retry attempt %d/%d...", attempt, maxRetries)
		}

		// Create a child context with timeout for this fetch attempt
		fetchCtx, fetchCancel := context.WithTimeout(ctxHolder.Get(), fetchTimeout)

		// Attempt to fetch complaints
		fetcher := complaint.New(fetchCtx, stor, tg, cfg, translator)
		activeComplaintIDs, err := fetcher.FetchAll(complaintURL)
		fetchCancel() // Always cancel timeout context

		if err == nil {
			// Success! Check for resolved complaints and update health
			markResolvedComplaints(ctxHolder.Get(), stor, tg, activeComplaintIDs)
			healthMonitor.UpdateFetchStatus("success")
			return nil
		}

		lastErr = err

		// Analyze error type and determine recovery strategy
		if sessionErr, ok := err.(*errors.SessionExpiredError); ok {
			// Session expired → Try re-login
			log.Println("🔄 Session expired:", sessionErr.Message)
			log.Println("🔐 Attempting re-login...")

			loginErr := auth.Login(ctxHolder.Get(), loginURL, username, password)
			if loginErr == nil {
				log.Println("✓ Re-login successful, retrying fetch on next loop...")
				continue
			}

			// Re-login failed → Restart browser
			log.Println("❌ Re-login failed:", loginErr)
			log.Println("🔄 Restarting browser context...")

			newCtx, newCancel := browser.NewContext()
			ctxHolder.Set(newCtx, newCancel)

			log.Println("🔐 Attempting login after browser restart...")
			loginErr2 := auth.Login(ctxHolder.Get(), loginURL, username, password)
			if loginErr2 == nil {
				log.Println("✓ Login successful after browser restart, retrying fetch on next loop...")
				continue
			}

			log.Println("❌ Login failed even after browser restart:", loginErr2)
		} else {
			// Generic error → Wait and retry
			log.Println("⚠️  Error fetching complaints:", err)
			time.Sleep(5 * time.Second)
		}
	}

	// All retry attempts failed
	log.Println("❌ All retry attempts failed.")

	// Update health monitor
	healthMonitor.UpdateFetchStatus(fmt.Sprintf("error: %v", lastErr))

	// Send critical alert to Telegram
	if tg != nil {
		log.Println("🚨 Sending critical failure alert...")
		alertErr := tg.SendCriticalAlert(
			"Fetch/Login Failure",
			fmt.Sprintf("Unable to fetch complaints after %d attempts. Last error: %v", maxRetries, lastErr),
			maxRetries,
		)
		if alertErr != nil {
			log.Println("⚠️  Failed to send Telegram alert:", alertErr)
		}
	}

	return fmt.Errorf("all %d retry attempts failed: %w", maxRetries, lastErr)
}

// markResolvedComplaints checks for complaints that were previously seen
// but are no longer on the website, and marks them as resolved in Telegram.
//
// Resolution detection logic:
//   - Complaint in storage + NOT in active list = Resolved
//   - Edit Telegram message to show "RESOLVED" status
//   - Remove from storage to stop tracking
//
// This handles automatic resolution detection when complaints are
// resolved on the website without using the Telegram button.
//
// Parameters:
//   - ctx: Browser context (not used currently, but available for future use)
//   - stor: Storage with previously seen complaints
//   - tg: Telegram client for editing messages
//   - activeIDs: List of currently active complaint IDs from website
func markResolvedComplaints(ctx context.Context, stor *storage.Storage, tg *telegram.Client, activeIDs []string) {
	// Create a map of currently active IDs for O(1) lookup
	activeIDsMap := make(map[string]bool)
	for _, id := range activeIDs {
		activeIDsMap[id] = true
	}

	// Get all previously seen complaints from storage
	allSeen := stor.GetAllSeenComplaints()

	resolvedCount := 0
	for _, complaintID := range allSeen {
		// If complaint is in storage but NOT in active list, it's resolved
		if !activeIDsMap[complaintID] {
			messageID := stor.GetMessageID(complaintID)
			if messageID != "" && tg != nil {
				log.Printf("✅ Marking complaint %s as resolved", complaintID)

				// Get consumer name from storage
				consumerName := stor.GetConsumerName(complaintID)
				if consumerName == "" {
					consumerName = "Unknown"
				}

				// Create resolved message
				resolvedMessage := fmt.Sprintf(
					"✅ <b>RESOLVED</b>\n\n"+
						"Complaint #%s\n"+
						"👤 %s\n"+
						"🕐 %s",
					complaintID,
					consumerName,
					time.Now().Format("02 Jan 2006, 03:04 PM"),
				)

				// Edit Telegram message
				err := tg.EditMessageText(tg.ChatID, messageID, resolvedMessage)
				if err != nil {
					log.Printf("⚠️  Failed to edit message for complaint %s: %v", complaintID, err)
				} else {
					// Remove from storage after successful edit
					if rmErr := stor.Remove(complaintID); rmErr != nil {
						log.Printf("⚠️  Failed to remove complaint %s from storage: %v", complaintID, rmErr)
					} else {
						log.Printf("✅ Removed resolved complaint %s from storage", complaintID)
						resolvedCount++
					}
				}
			}
		}
	}

	if resolvedCount > 0 {
		log.Printf("🎉 Marked %d complaints as resolved", resolvedCount)
	}
}