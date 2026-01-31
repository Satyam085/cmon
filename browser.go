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