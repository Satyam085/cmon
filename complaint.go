package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chromedp/chromedp"
)

func FetchComplaints(ctx context.Context, url string) error {
	log.Println("  → Navigating to complaints page...")
	var rows [][]string

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),

		// wait for table to load
		chromedp.WaitVisible("table", chromedp.ByQuery),

		// extract rows
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll("table tbody tr")).map(row =>
				Array.from(row.querySelectorAll("td")).map(td => td.innerText.trim())
			)
		`, &rows),
	)
	if err != nil {
		log.Println("  ✗ Failed to fetch complaints:", err)
		return err
	}
	log.Println("  ✓ Complaints page loaded")

	log.Println("📄 Complaints found:", len(rows))

	for i, row := range rows {
		fmt.Println("----- Complaint", i+1, "-----")
		for j, col := range row {
			fmt.Printf("Col %d: %s\n", j+1, col)
		}
	}

	return nil
}