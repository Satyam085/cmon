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
//  2. Login to DGVCL portal via HTTP (no browser required)
//  3. Initial fetch of complaints
//  4. Start periodic refresh loop (every 15 minutes by default)
//  5. Handle errors with retry logic and session reset
//  6. Graceful shutdown on SIGTERM/SIGINT
//
// Error recovery strategy:
//   - Session expired → Re-login
//   - Re-login failed → Reset session (new cookie jar) and re-login
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

	"cmon/internal/api"
	"cmon/internal/auth"
	"cmon/internal/complaint"
	"cmon/internal/config"
	"cmon/internal/errors"
	"cmon/internal/health"
	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/telegram"
	"cmon/internal/translate"
	"cmon/internal/whatsapp"
)

func main() {
	// Force Indian Standard Time (IST) for all time operations
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist

	log.Println("🚀 Starting CMON application...")

	// Step 1: Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}
	log.Printf("✓ Loaded credentials for user: %s", cfg.Username)
	log.Printf("✓ Pagination limit: %d pages", cfg.MaxPages)
	log.Printf("✓ Worker pool size: %d workers", cfg.WorkerPoolSize)

	// Step 1.5: Initialize shared API HTTP client
	api.InitHTTPClient(cfg)

	// Step 2: Initialize storage
	log.Println("📋 Initializing complaint storage...")
	stor := storage.New()

	// Step 3: Initialize Telegram client (optional)
	log.Println("📱 Initializing Telegram...")
	tg := telegram.NewClient()

	// Step 3a: Initialize WhatsApp client (optional)
	log.Println("💬 Initializing WhatsApp...")
	wa := whatsapp.NewClient()
	if wa != nil {
		defer wa.Disconnect()
	}

	// Step 3b: Initialize Gemini Translator (optional)
	log.Println("🌐 Initializing Gujarati translator...")
	translator, err := translate.NewTranslator(context.Background(), cfg.GeminiAPIKey, cfg)
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

	// Step 6: Create authenticated session client (replaces browser context)
	log.Println("🌐 Initializing HTTP session client...")
	sc, err := session.New()
	if err != nil {
		log.Fatal("❌ Failed to create session client:", err)
	}
	log.Println("✓ Session client created")

	// Step 7: Start Telegram callback handler if configured
	if tg != nil {
		callbackCtx, callbackCancel := context.WithCancel(context.Background())
		defer callbackCancel()

		go tg.HandleUpdates(callbackCtx, sc, stor)
		log.Println("✓ Telegram callback handler started")
	}

	// Step 7a: Start WhatsApp event handler if configured
	if wa != nil {
		waCtx, waCancel := context.WithCancel(context.Background())
		defer waCancel()

		go wa.HandleEvents(waCtx, sc, stor, cfg.WhatsAppResolveEnabled, cfg.DebugMode)
		if cfg.WhatsAppResolveEnabled {
			log.Println("✓ WhatsApp event handler started (resolve-by-reply ENABLED)")
		} else {
			log.Println("✓ WhatsApp event handler started (/summary only; resolve-by-reply disabled)")
		}
	}

	// Step 8: Login with retry logic
	log.Println("🔐 Attempting to login...")
	var loginErr error
	for attempt := 1; attempt <= cfg.MaxLoginRetries; attempt++ {
		log.Printf("   Login attempt %d/%d...", attempt, cfg.MaxLoginRetries)
		loginErr = auth.Login(sc, cfg.LoginURL, cfg.Username, cfg.Password)
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

	// Step 9: Initial fetch of complaints
	log.Println("📬 Fetching complaints...")
	fetcher := complaint.New(sc, stor, tg, wa, cfg, translator)
	activeComplaintIDs, err := fetcher.FetchAll(cfg.ComplaintURL)
	if err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}

	// Step 10: Check for resolved complaints
	markResolvedComplaints(stor, tg, wa, activeComplaintIDs)

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
			log.Println("✅ Cleanup complete, shutting down")
			return

		case <-ticker.C:
			log.Println("\n📬 Refreshing complaints list...")
			log.Println("⏰ Time:", time.Now().Format("2006-01-02 15:04:05"))

			fetchErr := fetchWithRetry(
				sc,
				cfg.ComplaintURL,
				stor,
				tg,
				wa,
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
//  1. Attempt fetch
//  2. If session expired → Re-login and retry
//  3. If re-login failed → Reset session (new cookie jar), re-login, and retry
//  4. Repeat up to maxRetries times
//  5. If all retries failed → Send critical alert
func fetchWithRetry(
	sc *session.Client,
	complaintURL string,
	stor *storage.Storage,
	tg *telegram.Client,
	wa *whatsapp.Client,
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

		fetcher := complaint.New(sc, stor, tg, wa, cfg, translator)
		activeComplaintIDs, err := fetcher.FetchAll(complaintURL)

		if err == nil {
			markResolvedComplaints(stor, tg, wa, activeComplaintIDs)
			healthMonitor.UpdateFetchStatus("success")
			return nil
		}

		lastErr = err

		if sessionErr, ok := err.(*errors.SessionExpiredError); ok {
			log.Println("🔄 Session expired:", sessionErr.Message)
			log.Println("🔐 Attempting re-login...")

			loginErr := auth.Login(sc, loginURL, username, password)
			if loginErr == nil {
				log.Println("✓ Re-login successful, retrying fetch on next loop...")
				continue
			}

			// Re-login failed → Reset session (equivalent of restarting browser)
			log.Println("❌ Re-login failed:", loginErr)
			log.Println("🔄 Resetting session (clearing cookies)...")

			if resetErr := sc.Reset(); resetErr != nil {
				log.Println("⚠️  Session reset failed:", resetErr)
			}

			log.Println("🔐 Attempting login after session reset...")
			loginErr2 := auth.Login(sc, loginURL, username, password)
			if loginErr2 == nil {
				log.Println("✓ Login successful after session reset, retrying fetch on next loop...")
				continue
			}

			log.Println("❌ Login failed even after session reset:", loginErr2)
		} else {
			log.Println("⚠️  Error fetching complaints:", err)
			time.Sleep(5 * time.Second)
		}
	}

	log.Println("❌ All retry attempts failed.")

	healthMonitor.UpdateFetchStatus(fmt.Sprintf("error: %v", lastErr))

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
func markResolvedComplaints(stor *storage.Storage, tg *telegram.Client, wa *whatsapp.Client, activeIDs []string) {
	activeIDsMap := make(map[string]bool)
	for _, id := range activeIDs {
		activeIDsMap[id] = true
	}

	allSeen := stor.GetAllSeenComplaints()

	resolvedCount := 0
	for _, complaintID := range allSeen {
		if !activeIDsMap[complaintID] {
			messageID := stor.GetMessageID(complaintID)
			if messageID != "" && tg != nil {
				log.Printf("✅ Marking complaint %s as resolved", complaintID)

				consumerName := stor.GetConsumerName(complaintID)
				if consumerName == "" {
					consumerName = "Unknown"
				}

				resolvedMessage := fmt.Sprintf(
					"✅ <b>RESOLVED</b>\n\n"+
						"Complaint #%s\n"+
						"👤 %s\n"+
						"🕐 %s",
					complaintID,
					consumerName,
					time.Now().Format("02 Jan 2006, 03:04 PM"),
				)

				err := tg.EditMessageText(tg.ChatID, messageID, resolvedMessage)
				if err != nil {
					log.Printf("⚠️  Failed to edit message for complaint %s: %v", complaintID, err)
				} else {
					if rmErr := stor.Remove(complaintID); rmErr != nil {
						log.Printf("⚠️  Failed to remove complaint %s from storage: %v", complaintID, rmErr)
					} else {
						log.Printf("✅ Removed resolved complaint %s from storage", complaintID)
						resolvedCount++
					}
				}

				if wa != nil {
					waResolvedMsg := fmt.Sprintf(
						"✅ RESOLVED\n\nComplaint #%s\n👤 %s\n🕐 %s",
						complaintID,
						consumerName,
						time.Now().Format("02 Jan 2006, 03:04 PM"),
					)
					if waErr := wa.SendMessage(waResolvedMsg); waErr != nil {
						log.Printf("⚠️  Failed to send WhatsApp resolved notice for %s: %v", complaintID, waErr)
					}
				}
			}
		}
	}

	if resolvedCount > 0 {
		log.Printf("🎉 Marked %d complaints as resolved", resolvedCount)
	}
}