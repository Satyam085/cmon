package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	log.Println("🚀 Starting CMON application...")
	
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  No .env file found or error loading it, reading from environment variables")
	} else {
		log.Println("✓ Loaded environment variables from .env file")
	}
	loginURL := "https://complaint.dgvcl.com/"
	complaintURL := "https://complaint.dgvcl.com/dashboard_complaint_list?from_date=&to_date=&honame=1&coname=21&doname=24&sdoname=87&cStatus=2&commobile="

	username := "2124087_technical"
	password := "dgvcl1234"

	log.Println("📋 Initializing complaint storage...")
	storage := NewComplaintStorage()

	log.Println("� Initializing Telegram...")
	telegramConfig := NewTelegramConfig()

	log.Println("�📋 Initializing browser context...")
	ctx, cancel := NewBrowserContext()
	defer cancel()
	log.Println("✓ Browser context created")

	// Login with retry logic
	maxLoginRetries := 3
	loginRetryDelay := 5 * time.Second
	
	log.Println("🔐 Attempting to login...")
	var loginErr error
	for attempt := 1; attempt <= maxLoginRetries; attempt++ {
		log.Printf("   Login attempt %d/%d...", attempt, maxLoginRetries)
		loginErr = Login(ctx, loginURL, username, password)
		if loginErr == nil {
			log.Println("✓ Login successful")
			break
		}
		
		if attempt < maxLoginRetries {
			log.Printf("   ❌ Login failed: %v", loginErr)
			log.Printf("   ⏳ Retrying in %v seconds...", loginRetryDelay.Seconds())
			time.Sleep(loginRetryDelay)
		}
	}
	
	if loginErr != nil {
		log.Fatal("❌ Login failed after", maxLoginRetries, "attempts:", loginErr)
	}

	log.Println("⏳ Waiting for page to load...")
	time.Sleep(2 * time.Second)

	// Initial fetch
	log.Println("📬 Fetching complaints...")
	_, err := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
	if err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}

	log.Println("✅ Initial fetch completed!")
	log.Println("⏰ Starting refresh loop - will check every 15 minutes...")
	log.Println("═══════════════════════════════════════════════════════════")

	// Refresh every 15 minutes
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("\n📬 Refreshing complaints list...")
		log.Println("⏰ Time:", time.Now().Format("2006-01-02 15:04:05"))
		
		// Attempt to fetch with full retry logic
		err := fetchWithRetry(ctx, cancel, &ctx, &cancel, complaintURL, storage, telegramConfig, loginURL, username, password)
		if err != nil {
			log.Println("⚠️  Final error after all retry attempts:", err)
			// Continue to next iteration - don't exit the loop
		}
		
		log.Println("═══════════════════════════════════════════════════════════")
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
func fetchWithRetry(ctx context.Context, cancel context.CancelFunc, ctxPtr *context.Context, cancelPtr *context.CancelFunc, 
	complaintURL string, storage *ComplaintStorage, telegramConfig *TelegramConfig, loginURL, username, password string) error {
	
	// First attempt to fetch
	newCount, err := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
	
	if err == nil {
		// Success!
		if len(newCount) == 0 {
			log.Println("✓ No new complaints")
		}
		return nil
	}
	
	// Check if it's a session expiration error
	sessionExpired := false
	if sessionErr, ok := err.(*SessionExpiredError); ok {
		log.Println("🔄 Session expired:", sessionErr.Message)
		sessionExpired = true
	} else {
		// Normal error - just log and return
		log.Println("⚠️  Error fetching complaints:", err)
		return err
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
				return nil
			}
			
			log.Println("⚠️  Fetch still failed after re-login:", retryErr)
			return retryErr
		}
		
		// Re-login failed - restart browser and try again
		log.Println("❌ Re-login failed:", loginErr)
		log.Println("🔄 Restarting browser context...")
		
		// Update the context pointers with new context
		newCtx, newCancel := RestartBrowserContext(cancel)
		*ctxPtr = newCtx
		*cancelPtr = newCancel
		
		log.Println("🔐 Attempting login after browser restart...")
		loginErr2 := Login(newCtx, loginURL, username, password)
		
		if loginErr2 == nil {
			log.Println("✓ Login successful after browser restart, retrying fetch...")
			
			// Retry fetch after successful re-login
			newCount, retryErr := FetchComplaints(newCtx, complaintURL, storage, telegramConfig)
			if retryErr == nil {
				log.Println("✓ Fetch successful after browser restart")
				if len(newCount) == 0 {
					log.Println("✓ No new complaints")
				}
				return nil
			}
			
			log.Println("⚠️  Fetch failed after browser restart:", retryErr)
			return retryErr
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
		
		return fmt.Errorf("all retry attempts failed: %w", loginErr2)
	}
	
	return err
}