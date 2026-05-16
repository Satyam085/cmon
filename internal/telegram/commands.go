package telegram

import (
	"context"
	"fmt"
	"log"

	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/summary"
)

// This file holds the per-command handlers dispatched by handleMessage in
// client.go. Splitting them out keeps client.go focused on transport and
// lifecycle (HTTP, long-poll, message routing) while command-specific logic
// (rendering, parsing, validation) lives here.

// PostScheduledSummary triggers the /summary flow as if a user had typed it
// in the chat. Exposed for the scheduler in main.go; never call it from a
// user-message handler (those go through the existing dispatch).
func (c *Client) PostScheduledSummary(ctx context.Context, sc *session.Client, stor *storage.Storage) {
	c.handleSummaryCommand(ctx, sc, stor)
}

// handleSummaryCommand processes the /summary command — fetches all pending
// complaints and sends a single combined PNG summary back to the chat.
func (c *Client) handleSummaryCommand(ctx context.Context, sc *session.Client, stor *storage.Storage) {
	log.Println("📊 /summary command received")

	processingMsg := Message{
		ChatID:    c.ChatID,
		Text:      "📊 <b>Generating summary...</b>\nFetching details for all pending complaints.",
		ParseMode: "HTML",
	}
	c.doRequest("sendMessage", processingMsg)

	// Fetch all pending complaint details
	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		log.Printf("⚠️  Summary fetch failed: %v\n", err)
		noDataMsg := Message{
			ChatID:    c.ChatID,
			Text:      "ℹ️ No pending complaints found.",
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", noDataMsg)
		return
	}

	// Render combined table image
	imgBytes, err := summary.RenderTable(complaints)
	if err != nil {
		log.Printf("⚠️  Summary render failed: %v\n", err)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Failed to render summary image: %v", err),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	caption := fmt.Sprintf("📋 %d Pending Complaints", len(complaints))
	if err := c.SendPhoto(c.ChatID, imgBytes, caption); err != nil {
		log.Printf("⚠️  Failed to send summary photo: %v\n", err)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Failed to send summary image: %v", err),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	log.Println("✓ Summary image sent successfully")
}
