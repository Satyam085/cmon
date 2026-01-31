package main

import (
	"context"
	"log"

	"github.com/chromedp/chromedp"
)

func NewBrowserContext() (context.Context, context.CancelFunc) {
	log.Println("  → Creating new browser context...")
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		chromedp.WithLogf(log.Printf),
	)
	log.Println("  ✓ Browser context created successfully")
	return ctx, cancel
}

// RestartBrowserContext cancels the old context and creates a new one
func RestartBrowserContext(oldCancel context.CancelFunc) (context.Context, context.CancelFunc) {
	log.Println("  ⚠️  Restarting browser context...")
	
	// Cancel old context
	if oldCancel != nil {
		oldCancel()
		log.Println("  ✓ Old browser context cancelled")
	}
	
	// Create new context
	return NewBrowserContext()
}