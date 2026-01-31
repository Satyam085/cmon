package main

import (
	"log"
	"time"
)

func main() {
	log.Println("🚀 Starting CMON application...")
	
	loginURL := "https://complaint.dgvcl.com/"
	complaintURL := "https://complaint.dgvcl.com/dashboard_complaint_list?from_date=&to_date=&honame=1&coname=21&doname=24&sdoname=87&cStatus=2&commobile="

	username := "2124087_technical"
	password := "dgvcl1234"

	log.Println("📋 Initializing browser context...")
	ctx, cancel := NewBrowserContext()
	defer cancel()
	log.Println("✓ Browser context created")

	log.Println("🔐 Attempting to login...")
	if err := Login(ctx, loginURL, username, password); err != nil {
		log.Fatal("❌ Login failed:", err)
	}

	log.Println("⏳ Waiting for page to load...")
	time.Sleep(2 * time.Second)

	log.Println("📬 Fetching complaints...")
	if err := FetchComplaints(ctx, complaintURL); err != nil {
		log.Fatal("❌ Failed to fetch complaints:", err)
	}

	log.Println("✅ Application completed successfully!")
	select {} // keep session alive
}