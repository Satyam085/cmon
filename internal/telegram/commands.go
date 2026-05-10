package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"cmon/internal/belt"
	"cmon/internal/complaintid"
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

// handleSummaryBeltCommand processes the /summarybelt command, sending one
// image per belt instead of a single combined image.
func (c *Client) handleSummaryBeltCommand(ctx context.Context, sc *session.Client, stor *storage.Storage) {
	log.Println("📊 /summarybelt command received")

	processingMsg := Message{
		ChatID:    c.ChatID,
		Text:      "📊 <b>Generating belt-wise summary...</b>\nRendering one image per belt.",
		ParseMode: "HTML",
	}
	c.doRequest("sendMessage", processingMsg)

	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		log.Printf("⚠️  Belt summary fetch failed: %v\n", err)
		noDataMsg := Message{
			ChatID:    c.ChatID,
			Text:      "ℹ️ No pending complaints found.",
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", noDataMsg)
		return
	}

	beltImages, err := summary.RenderTablesByBelt(complaints)
	if err != nil {
		log.Printf("⚠️  Belt summary render failed: %v\n", err)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Failed to render belt summary images: %v", err),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	for _, bi := range beltImages {
		caption := fmt.Sprintf("📋 %s Belt — %d Pending Complaints", bi.Label, bi.Count)
		if err := c.SendPhoto(c.ChatID, bi.PNG, caption); err != nil {
			log.Printf("⚠️  Failed to send %s belt summary photo: %v\n", bi.Label, err)
			errorMsg := Message{
				ChatID:    c.ChatID,
				Text:      fmt.Sprintf("❌ Failed to send %s belt summary image: %v", bi.Label, err),
				ParseMode: "HTML",
			}
			c.doRequest("sendMessage", errorMsg)
			continue
		}
	}

	log.Printf("✓ Belt summary sent (%d belt images, %d total complaints)\n",
		len(beltImages), len(complaints))
}

// handleMoveCommand processes the /move command. Two invocation forms:
//   - Reply to a complaint message:   /move <belt-name>
//   - With explicit complaint ID:     /move <complaint_id> <belt-name>
//
// Bad / missing arguments fall through to sendMoveUsage, which lists every
// valid belt name so the operator can copy-paste.
func (c *Client) handleMoveCommand(message *IncomingMessage, stor *storage.Storage) {
	text := strings.TrimSpace(message.Text)
	args := strings.Fields(text)

	var complaintID string
	var beltInput string

	switch {
	case len(args) >= 2 && message.ReplyToMessage != nil:
		complaintID = extractComplaintIDFromText(message.ReplyToMessage.Text)
		beltInput = strings.TrimSpace(strings.TrimPrefix(text, args[0]))
	case len(args) >= 3:
		complaintID = strings.TrimSpace(args[1])
		beltInput = strings.TrimSpace(strings.Join(args[2:], " "))
	default:
		c.sendMoveUsage()
		return
	}

	if complaintID == "" {
		c.sendTextMessage(
			fmt.Sprintf(
				"❌ Could not find the complaint number.\n\n"+
					"Reply to a complaint message with <code>/move belt-name</code>, or send <code>/move complaint_id belt-name</code>.\n\n"+
					"Valid belts: <code>%s</code>",
				strings.Join(belt.All(), ", "),
			),
			"HTML",
		)
		return
	}

	newBelt, ok := belt.Canonicalize(beltInput)
	if !ok {
		c.sendTextMessage(fmt.Sprintf("❌ Unknown belt <b>%s</b>.\nValid belts: <code>%s</code>", htmlEscape(strings.TrimSpace(beltInput)), strings.Join(belt.All(), ", ")), "HTML")
		return
	}

	if !stor.Exists(complaintID) {
		c.sendTextMessage(fmt.Sprintf("❌ Complaint <b>%s</b> is not in active storage.", htmlEscape(complaintID)), "HTML")
		return
	}

	oldBelt := belt.DisplayName(stor.GetBelt(complaintID))
	if err := stor.UpdateBelt(complaintID, newBelt); err != nil {
		log.Printf("⚠️  Failed to move complaint %s to %s: %v\n", complaintID, newBelt, err)
		c.sendTextMessage(fmt.Sprintf("❌ Failed to update complaint <b>%s</b>.", htmlEscape(complaintID)), "HTML")
		return
	}

	if message.ReplyToMessage != nil && message.ReplyToMessage.Text != "" {
		updatedText, changed := rewriteComplaintBeltLine(message.ReplyToMessage.Text, newBelt)
		if changed {
			_, err := c.doRequest("editMessageText", EditMessageRequest{
				ChatID:      c.ChatID,
				MessageID:   fmt.Sprintf("%d", message.ReplyToMessage.MessageID),
				Text:        updatedText,
				ParseMode:   "HTML",
				ReplyMarkup: nil,
			})
			if err != nil {
				log.Printf("⚠️  Failed to edit complaint message for %s after move: %v\n", complaintID, err)
			}
		}
	}

	c.sendTextMessage(fmt.Sprintf("✅ Complaint <b>%s</b> moved from <b>%s</b> to <b>%s</b>.", htmlEscape(complaintID), htmlEscape(oldBelt), htmlEscape(newBelt)), "HTML")
}

func (c *Client) sendMoveUsage() {
	belts := belt.All()
	example := "dahod"
	if len(belts) > 0 {
		example = belts[0]
	}
	c.sendTextMessage(
		fmt.Sprintf(
			"<b>Move a complaint to a different belt</b>\n\n"+
				"Usage:\n"+
				"• Reply to a complaint message with <code>/move belt-name</code>\n"+
				"• Or send <code>/move complaint_id belt-name</code>\n\n"+
				"Example: <code>/move 12345678 %s</code>\n\n"+
				"Valid belts: <code>%s</code>",
			example,
			strings.Join(belts, ", "),
		),
		"HTML",
	)
}

// sendTextMessage is a thin convenience for the command handlers that need
// to push a plain text reply without crafting a full Message struct.
func (c *Client) sendTextMessage(text, parseMode string) {
	msg := Message{
		ChatID:    c.ChatID,
		Text:      text,
		ParseMode: parseMode,
	}
	c.doRequest("sendMessage", msg)
}

// isMoveCommand reports whether the first whitespace-delimited token of text
// is exactly "/move". Used by handleMessage to dispatch.
func isMoveCommand(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return false
	}

	return fields[0] == "/move"
}

// extractComplaintIDFromText forwards to complaintid.FromText. Kept as a
// package-local alias so existing call sites read naturally.
func extractComplaintIDFromText(text string) string {
	return complaintid.FromText(text)
}

// rewriteComplaintBeltLine swaps the "Belt:" line in a previously-sent
// complaint message with one for the new belt (emoji + display name). The
// bool reports whether a Belt: line was found; if false, text is returned
// unchanged so /move falls back to a simple acknowledgement.
func rewriteComplaintBeltLine(text, beltName string) (string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.Contains(line, " Belt: ") {
			lines[i] = fmt.Sprintf("%s Belt: %s", belt.StyleFor(beltName).Emoji, belt.DisplayName(beltName))
			return strings.Join(lines, "\n"), true
		}
	}

	return text, false
}

// htmlEscape escapes the three characters Telegram's HTML parse mode treats
// specially. Used by command handlers when interpolating untrusted input
// (complaint IDs from user replies, belt names, etc.) into messages.
func htmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(value)
}
