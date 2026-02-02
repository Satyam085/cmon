package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"
)

var (
	startTime       time.Time
	lastFetchTime   time.Time
	lastFetchStatus string
	stateMu         sync.RWMutex
)

func main() {

	// Force Indian Standard Time (IST) for all time operations
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist

	// Application start

	startTime = time.Now()
	log.Println("🚀 Starting CMON application...")

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}
	log.Printf("✓ Loaded credentials for user: %s", cfg.Username)
	log.Printf("✓ Pagination limit: %d pages", cfg.MaxPages)

	log.Println("📋 Initializing complaint storage...")
	storage := NewComplaintStorage()

	log.Println("📱 Initializing Telegram...")
	telegramConfig := NewTelegramConfig()

	// Start health check server in background
	go startHealthCheckServer(cfg.HealthCheckPort)
	log.Printf("✓ Health check server started on :%s", cfg.HealthCheckPort)

	log.Println("🌐 Initializing browser context...")
	ctxHolder := NewBrowserContextHolder()
	defer ctxHolder.Cancel()
	log.Println("✓ Browser context created")

	// Start Telegram callback handler if Telegram is configured
	if telegramConfig != nil {
		// Create a context for the callback handler
		callbackCtx, callbackCancel := context.WithCancel(context.Background())
		defer callbackCancel()
		
		go telegramConfig.HandleUpdates(callbackCtx, ctxHolder, storage)
		log.Println("✓ Telegram callback handler started")
	}

	// Login with retry logic
	log.Println("🔐 Attempting to login...")
	var loginErr error
	for attempt := 1; attempt <= cfg.MaxLoginRetries; attempt++ {
		log.Printf("   Login attempt %d/%d...", attempt, cfg.MaxLoginRetries)
		loginErr = Login(ctxHolder.Get(), cfg.LoginURL, cfg.Username, cfg.Password)
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

	// Initial fetch
	log.Println("📬 Fetching complaints...")
	activeComplaintIDs, err := FetchComplaints(ctxHolder.Get(), cfg.ComplaintURL, storage, telegramConfig, cfg)
	if err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}

	// Check for resolved complaints (compare finding vs storage)
	markResolvedComplaints(ctxHolder.Get(), storage, telegramConfig, activeComplaintIDs)

	lastFetchTime = time.Now()
	lastFetchStatus = "success"

	log.Println("✅ Initial fetch completed!")
	log.Printf("⏰ Starting refresh loop - will check every %v...\n", cfg.FetchInterval)
	log.Println("═══════════════════════════════════════════════════════════")

	// Set up graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Refresh ticker
	ticker := time.NewTicker(cfg.FetchInterval)
	defer ticker.Stop()

	// Main loop with graceful shutdown
	for {
		select {
		case <-shutdownCtx.Done():
			log.Println("\n🛑 Shutdown signal received, cleaning up...")
			ctxHolder.Cancel() // Cancel browser context
			log.Println("✅ Cleanup complete, shutting down")
			return
		case <-ticker.C:
			// Fall through to execution logic below
		}

		log.Println("\n📬 Refreshing complaints list...")
		log.Println("⏰ Time:", time.Now().Format("2006-01-02 15:04:05"))

		// Attempt to fetch with full retry logic
		var fetchErr error
		fetchErr = fetchWithRetry(ctxHolder, cfg.ComplaintURL, storage, telegramConfig, cfg.LoginURL, cfg.Username, cfg.Password, cfg, cfg.MaxFetchRetries, cfg.FetchTimeout)
		
		stateMu.Lock()
		if fetchErr != nil {
			log.Println("⚠️  Final error after all retry attempts:", fetchErr)
			lastFetchStatus = fmt.Sprintf("error: %v", fetchErr)
		} else {
			lastFetchTime = time.Now()
			lastFetchStatus = "success"
		}
		stateMu.Unlock()

		log.Println("═══════════════════════════════════════════════════════════")
	}
}

// fetchWithRetry implements the complete error handling flow using configuration
func fetchWithRetry(ctxHolder *BrowserContextHolder,
	complaintURL string, storage *ComplaintStorage, telegramConfig *TelegramConfig, 
	loginURL, username, password string, cfg *Config, maxRetries int, fetchTimeout time.Duration) error {

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔄 Retry attempt %d/%d...", attempt, maxRetries)
		}

		// 1. Attempt to fetch with timeout
		// Create a child context with timeout for this specific fetch attempt
		fetchCtx, fetchCancel := context.WithTimeout(ctxHolder.Get(), fetchTimeout)
		
		activeComplaintIDs, err := FetchComplaints(fetchCtx, complaintURL, storage, telegramConfig, cfg)
		fetchCancel() // Always cancel the timeout context when done

		if err == nil {
			// Success!
			markResolvedComplaints(ctxHolder.Get(), storage, telegramConfig, activeComplaintIDs)
			return nil
		}
		
		lastErr = err

		// 2. Analyze Error
		sessionExpired := false
		if sessionErr, ok := err.(*SessionExpiredError); ok {
			log.Println("🔄 Session expired:", sessionErr.Message)
			sessionExpired = true
		} else {
			log.Println("⚠️  Error fetching complaints:", err)
			// If it's not a session error, we still retry, assuming transient network issue?
			// Or should we only retry on Session Expired?
			// For now, we'll try to recover from session expiry primarily, 
			// but maybe a simple timeout also deserves a generic retry.
			// If it's NOT session expired, we just loop to next attempt (maybe sleep?)
			// If it IS session expired, we try login flow.
		}

		if !sessionExpired {
			// Generic error, wait a bit and retry loop
			time.Sleep(5 * time.Second)
			continue
		}

		// 3. Recovery: Session Expired
		log.Println("🔐 Attempting re-login...")
		loginErr := Login(ctxHolder.Get(), loginURL, username, password)

		if loginErr == nil {
			log.Println("✓ Re-login successful, retrying fetch on next loop...")
			continue
		}

		// 4. Recovery: Login Failed -> Restart Browser
		log.Println("❌ Re-login failed:", loginErr)
		log.Println("🔄 Restarting browser context...")

		newCtx, newCancel := NewBrowserContext()
		ctxHolder.Set(newCtx, newCancel)

		log.Println("🔐 Attempting login after browser restart...")
		loginErr2 := Login(ctxHolder.Get(), loginURL, username, password)
		if loginErr2 == nil {
			log.Println("✓ Login successful after browser restart, retrying fetch on next loop...")
			continue
		}
		
		log.Println("❌ Login failed even after browser restart:", loginErr2)
		// If even browser restart + login failed, we are in trouble.
		// We use up one "attempt" of the outer loop.
	}

	// All retry attempts failed - send Telegram alert
	log.Println("❌ All retry attempts failed.")
	log.Println("🚨 Sending critical failure alert...")

	alertErr := telegramConfig.SendCriticalAlert(
		"Fetch/Login Failure",
		fmt.Sprintf("Unable to fetch complaints after %d attempts. Last error: %v", maxRetries, lastErr),
		maxRetries,
	)

	if alertErr != nil {
		log.Println("⚠️  Failed to send Telegram alert:", alertErr)
	}

	return fmt.Errorf("all %d retry attempts failed: %w", maxRetries, lastErr)
}

// markResolvedComplaints checks for complaints that were previously seen
// but are no longer on the website (in the first MaxPages), and marks them as resolved in Telegram
func markResolvedComplaints(ctx context.Context, storage *ComplaintStorage, telegramConfig *TelegramConfig, activeIDs []string) {
	// 1. Create a map of currently active IDs for O(1) lookup
	activeIDsMap := make(map[string]bool)
	for _, id := range activeIDs {
		activeIDsMap[id] = true
	}

	// 2. Get all previously seen complaints from local storage
	allSeen := storage.GetAllSeenComplaints()

	resolvedCount := 0
	for _, complaintID := range allSeen {
		// 3. Logic: If a complaint is in Storage, but NOT in the Active list found on the site
		// It implies it has been resolved
		if !activeIDsMap[complaintID] {
			messageID := storage.GetMessageID(complaintID)
			if messageID != "" && telegramConfig != nil {
				log.Printf("✅ Marking complaint %s as resolved", complaintID)

				resolvedTime := time.Now().Format("02-01-2006 15:04:05")
				resolvedMessage := fmt.Sprintf(
					"<b>✅ RESOLVED</b>\n"+
						"━━━━━━━━━━━━━━━━━━━━\n\n"+
						"<s>Complaint No: %s</s>\n"+
						"<s>This complaint has been resolved.</s>\n\n"+
						"🕒 <b>Resolved At:</b> %s\n"+
						"━━━━━━━━━━━━━━━━━━━━",
					complaintID,
					resolvedTime,
				)

				err := telegramConfig.EditMessageText(telegramConfig.ChatID, messageID, resolvedMessage)
				if err != nil {
					log.Printf("⚠️  Failed to edit message for complaint %s: %v", complaintID, err)
				} else {
					// Remove from storage after successful edit to stop tracking it
					if rmErr := storage.Remove(complaintID); rmErr != nil {
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

// Health check types and handler

type HealthStatus struct {
	Status          string `json:"status"`
	Uptime          string `json:"uptime"`
	LastFetchTime   string `json:"last_fetch_time"`
	LastFetchStatus string `json:"last_fetch_status"`
}

func startHealthCheckServer(port string) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		stateMu.RLock()
		defer stateMu.RUnlock()
		
		uptime := time.Since(startTime)

		status := HealthStatus{
			Status:          "healthy",
			Uptime:          uptime.String(),
			LastFetchTime:   lastFetchTime.Format("2006-01-02 15:04:05"),
			LastFetchStatus: lastFetchStatus,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(status)
	})

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Printf("⚠️  Health check server error: %v", err)
	}
}