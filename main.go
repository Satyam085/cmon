package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

var (
	startTime time.Time
	lastFetchTime time.Time
	lastFetchStatus string
)

func main() {
	startTime = time.Now()
	log.Println("🚀 Starting CMON application...")
	
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  No .env file found or error loading it, reading from environment variables")
	} else {
		log.Println("✓ Loaded environment variables from .env file")
	}

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}
	log.Printf("✓ Loaded credentials for user: %s", cfg.Username)

	log.Println("📋 Initializing complaint storage...")
	storage := NewComplaintStorage()

	log.Println("📱 Initializing Telegram...")
	telegramConfig := NewTelegramConfig()

	// Start health check server in background
	go startHealthCheckServer(cfg.HealthCheckPort)
	log.Printf("✓ Health check server started on :%s", cfg.HealthCheckPort)

	log.Println("🌐 Initializing browser context...")
	ctx, cancel := NewBrowserContext()
	defer cancel()
	log.Println("✓ Browser context created")

	// Login with retry logic
	log.Println("🔐 Attempting to login...")
	var loginErr error
	for attempt := 1; attempt <= cfg.MaxLoginRetries; attempt++ {
		log.Printf("   Login attempt %d/%d...", attempt, cfg.MaxLoginRetries)
		loginErr = Login(ctx, cfg.LoginURL, cfg.Username, cfg.Password)
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
	_, err = FetchComplaints(ctx, cfg.ComplaintURL, storage, telegramConfig)
	if err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}
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
			cancel() // Cancel browser context
			log.Println("✅ Cleanup complete, shutting down")
			return

		case <-ticker.C:
			log.Println("\n📬 Refreshing complaints list...")
			log.Println("⏰ Time:", time.Now().Format("2006-01-02 15:04:05"))
			
			// Attempt to fetch with full retry logic
			var fetchErr error
			ctx, cancel, fetchErr = fetchWithRetry(ctx, cancel, cfg.ComplaintURL, storage, telegramConfig, cfg.LoginURL, cfg.Username, cfg.Password)
			if fetchErr != nil {
				log.Println("⚠️  Final error after all retry attempts:", fetchErr)
				lastFetchStatus = fmt.Sprintf("error: %v", fetchErr)
				// Continue to next iteration - don't exit the loop
			} else {
				lastFetchTime = time.Now()
				lastFetchStatus = "success"
			}
			
			log.Println("═══════════════════════════════════════════════════════════")
		}
	}
}

// fetchWithRetry implements the complete error handling flow:
// Fetch fails
//   ├─ normal error → log & continue
//   ├─ session expired
//   │   ├─ re-login succeeds → retry fetch
//   │   └─ re-login fails
//   │       ├─ restart browser
//   │       ├─ re-login again
//   │       └─ if still fails → Telegram alert
// Returns: (newContext, newCancelFunc, error)
func fetchWithRetry(ctx context.Context, cancel context.CancelFunc,
	complaintURL string, storage *ComplaintStorage, telegramConfig *TelegramConfig, loginURL, username, password string) (context.Context, context.CancelFunc, error) {
	
	// First attempt to fetch
	newCount, err := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
	
	if err == nil {
		// Success!
		if len(newCount) == 0 {
			log.Println("✓ No new complaints")
		}
		return ctx, cancel, nil
	}
	
	// Check if it's a session expiration error
	sessionExpired := false
	if sessionErr, ok := err.(*SessionExpiredError); ok {
		log.Println("🔄 Session expired:", sessionErr.Message)
		sessionExpired = true
	} else {
		// Normal error - just log and return
		log.Println("⚠️  Error fetching complaints:", err)
		return ctx, cancel, err
	}
	
	// Session expired - attempt re-login
	if sessionExpired {
		log.Println("🔐 Attempting re-login...")
		loginErr := Login(ctx, loginURL, username, password)
		
		if loginErr == nil {
			log.Println("✓ Re-login successful, retrying fetch...")
			
			// Retry fetch after successful re-login
			newCount, retryErr := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
			if retryErr == nil {
				log.Println("✓ Fetch successful after re-login")
				if len(newCount) == 0 {
					log.Println("✓ No new complaints")
				}
				return ctx, cancel, nil
			}
			
			log.Println("⚠️  Fetch still failed after re-login:", retryErr)
			return ctx, cancel, retryErr
		}
		
		// Re-login failed - restart browser and try again
		log.Println("❌ Re-login failed:", loginErr)
		log.Println("🔄 Restarting browser context...")
		
		// Restart browser context
		ctx, cancel = RestartBrowserContext(cancel)
		
		log.Println("🔐 Attempting login after browser restart...")
		loginErr2 := Login(ctx, loginURL, username, password)
		
		if loginErr2 == nil {
			log.Println("✓ Login successful after browser restart, retrying fetch...")
			
			// Retry fetch after successful re-login
			newCount, retryErr := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
			if retryErr == nil {
				log.Println("✓ Fetch successful after browser restart")
				if len(newCount) == 0 {
					log.Println("✓ No new complaints")
				}
				return ctx, cancel, nil
			}
			
			log.Println("⚠️  Fetch failed after browser restart:", retryErr)
			return ctx, cancel, retryErr
		}
		
		// All retry attempts failed - send Telegram alert
		log.Println("❌ All retry attempts failed:", loginErr2)
		log.Println("🚨 Sending critical failure alert...")
		
		alertErr := telegramConfig.SendCriticalAlert(
			"Login Failure After Browser Restart",
			fmt.Sprintf("Unable to login after browser restart. Last error: %v", loginErr2),
			3, // Total retry attempts: initial login, re-login, login after restart
		)
		
		if alertErr != nil {
			log.Println("⚠️  Failed to send Telegram alert:", alertErr)
		}
		
		return ctx, cancel, fmt.Errorf("all retry attempts failed: %w", loginErr2)
	}
	
	return ctx, cancel, err
}

// Health check types and handler

type HealthStatus struct {
	Status        string    `json:"status"`
	Uptime        string    `json:"uptime"`
	LastFetchTime string    `json:"last_fetch_time"`
	LastFetchStatus string  `json:"last_fetch_status"`
}

func startHealthCheckServer(port string) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime)
		
		status := HealthStatus{
			Status:        "healthy",
			Uptime:        uptime.String(),
			LastFetchTime: lastFetchTime.Format("2006-01-02 15:04:05"),
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