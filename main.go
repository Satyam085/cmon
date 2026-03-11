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

	log.Println("🚀 Starting CMON...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}


	// Initialize storage
	stor := storage.New()
	defer func() {
		if err := stor.Close(); err != nil {
			log.Printf("⚠️  Failed to close database: %v", err)
		}
	}()

	// Step 3: Initialize Telegram client (optional)
	tg := telegram.NewClient()

	// Step 3a: Initialize WhatsApp client (optional)
	wa := whatsapp.NewClient()
	if wa != nil {
		defer wa.Disconnect()
	}

	// Step 3b: Initialize Gemini Translator (optional)
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
	}

	// Step 7a: Start WhatsApp event handler if configured
	if wa != nil {
		waCtx, waCancel := context.WithCancel(context.Background())
		defer waCancel()
		go wa.HandleEvents(waCtx, sc, stor, cfg.WhatsAppResolveEnabled, cfg.DebugMode)
	}

	log.Println("🔐 Logging in...")
	var loginErr error
	for attempt := 1; attempt <= cfg.MaxLoginRetries; attempt++ {
		loginErr = auth.Login(sc, cfg.LoginURL, cfg.Username, cfg.Password)
		if loginErr == nil {
			log.Println("✓ Logged in")
			break
		}

		if attempt < cfg.MaxLoginRetries {
			log.Printf("   ❌ Login failed: %v", loginErr)
			log.Printf("   ⏳ Retrying in %v...", cfg.LoginRetryDelay)
			time.Sleep(cfg.LoginRetryDelay)
		}
	}

	if loginErr != nil {
		log.Fatal("❌ Login failed:", loginErr)
	}

	log.Println("📬 Fetching complaints...")
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
		log.Fatal("❌ Failed initial fetch after all retries:", fetchErr)
	}

	healthMonitor.UpdateFetchStatus("success")

	log.Printf("⏰ Running — next check in %v\n", cfg.FetchInterval)
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
			log.Printf("📬 Refreshing — %s", time.Now().Format("15:04:05"))

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