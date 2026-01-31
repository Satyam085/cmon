package main

import (
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

	log.Println("🔐 Attempting to login...")
	if err := Login(ctx, loginURL, username, password); err != nil {
		log.Fatal("❌ Login failed:", err)
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
		
		newCount, err := FetchComplaints(ctx, complaintURL, storage, telegramConfig)
		if err != nil {
			log.Println("⚠️  Error fetching complaints:", err)
			continue
		}
		
		if len(newCount) == 0 {
			log.Println("✓ No new complaints")
		}
		log.Println("═══════════════════════════════════════════════════════════")
	}
}