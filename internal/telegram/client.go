// Package telegram provides Telegram bot integration for the CMON application.
//
// This package handles:
//   - Sending complaint notifications with inline keyboards
//   - Receiving and processing callback queries (button clicks)
//   - Handling user messages (resolution notes)
//   - Editing messages to mark complaints as resolved
//   - Long polling for updates
//
// Architecture:
//   - Client: Main struct with bot token and chat ID
//   - Update handler: Background goroutine for long polling
//   - Callback handler: Processes button clicks
//   - Message handler: Processes text messages (resolution notes)
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"cmon/internal/api"
	"cmon/internal/storage"
)

// PendingResolution stores information about a complaint awaiting resolution note.
//
// When a user clicks "Mark as Resolved" button:
//   1. Store complaint info in pendingResolutions map
//   2. Send prompt message asking for resolution note
//   3. Wait for user's reply
//   4. Process reply and mark complaint as resolved
//
// Fields:
//   - ComplaintNumber: Complaint ID being resolved
//   - MessageID: Telegram message ID to edit after resolution
//   - OriginalText: Original message text (to extract consumer name)
//   - PromptMessageID: ID of prompt message (to delete after reply)
type PendingResolution struct {
	ComplaintNumber string
	MessageID       string
	OriginalText    string
	PromptMessageID int
}

// Client represents a Telegram bot client.
//
// Thread-safety:
//   - pendingResolutions map is protected by mutex
//   - Safe for concurrent access from update handler and main thread
//
// Fields:
//   - BotToken: Telegram bot API token
//   - ChatID: Target chat ID for notifications
//   - pendingResolutions: Map of user ID to pending resolution
//   - DebugMode: If true, skip actual API calls (for testing)
type Client struct {
	BotToken           string
	ChatID             string
	mu                 sync.Mutex
	pendingResolutions map[int64]PendingResolution
	DebugMode          bool
}

// Message types for Telegram API

// Message represents a Telegram message for sending.
type Message struct {
	ChatID                string      `json:"chat_id"`
	Text                  string      `json:"text"`
	ParseMode             string      `json:"parse_mode"`
	DisableWebPagePreview bool        `json:"disable_web_page_preview"`
	ReplyMarkup           interface{} `json:"reply_markup,omitempty"`
	ReplyToMessageID      int         `json:"reply_to_message_id,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton represents a button in an inline keyboard.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// ForceReply prompts user to reply to the bot's message.
type ForceReply struct {
	ForceReply            bool   `json:"force_reply"`
	Selective             bool   `json:"selective,omitempty"`
	InputFieldPlaceholder string `json:"input_field_placeholder,omitempty"`
}

// Update represents a Telegram update from getUpdates.
type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *IncomingMessage `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// IncomingMessage represents a received Telegram message.
type IncomingMessage struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      *Chat  `json:"chat,omitempty"`
	Text      string `json:"text"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// CallbackQuery represents a callback query from an inline button.
type CallbackQuery struct {
	ID      string           `json:"id"`
	From    User             `json:"from"`
	Message *IncomingMessage `json:"message"`
	Data    string           `json:"data"`
}

// User represents a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// EditMessageRequest represents a request to edit a message.
type EditMessageRequest struct {
	ChatID      string                `json:"chat_id"`
	MessageID   string                `json:"message_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// NewClient creates a new Telegram client from environment variables.
//
// Configuration:
//   - TELEGRAM_BOT_TOKEN: Bot API token from @BotFather
//   - TELEGRAM_CHAT_ID: Target chat ID for notifications
//   - DEBUG_MODE: If "true", skip actual API calls
//
// Returns:
//   - *Client: Configured Telegram client, or nil if not configured
func NewClient() *Client {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if botToken == "" || chatID == "" {
		log.Println("⚠️  TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set. Telegram notifications disabled.")
		if botToken == "" {
			log.Println("   → Missing: TELEGRAM_BOT_TOKEN")
		}
		if chatID == "" {
			log.Println("   → Missing: TELEGRAM_CHAT_ID")
		}
		return nil
	}

	log.Println("✓ Telegram configured successfully")

	debugMode := os.Getenv("DEBUG_MODE") == "true"
	if debugMode {
		log.Println("🐛 DEBUG MODE ENABLED - API calls will be simulated")
	}

	return &Client{
		BotToken:           botToken,
		ChatID:             chatID,
		pendingResolutions: make(map[int64]PendingResolution),
		DebugMode:          debugMode,
	}
}

// doRequest handles the common logic for sending requests to Telegram API.
//
// Features:
//   - JSON marshaling
//   - HTTP POST with proper headers
//   - Error response parsing
//   - Long timeout for long polling (60s)
//
// Parameters:
//   - method: Telegram API method name (e.g., "sendMessage")
//   - payload: Request payload (will be JSON marshaled)
//
// Returns:
//   - map[string]interface{}: Parsed response
//   - error: Request or API error
func (c *Client) doRequest(method string, payload interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.BotToken, method)

	// Use custom HTTP client with longer timeout for long polling
	// Standard timeout is 30s, but long polling needs 60s (30s poll + 30s overhead)
	telegramClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := telegramClient.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if API call succeeded
	if ok, exists := result["ok"].(bool); !exists || !ok {
		return nil, fmt.Errorf("Telegram API error: %v", result)
	}

	return result, nil
}

// SendComplaintMessage sends a new complaint notification to Telegram.
//
// Message format:
//   📋 Complaint : 12345
//   👤 John Doe
//   📞 9876543210
//   🆔 Consumer: 67890
//   📅 2026-01-15
//   💬 Details:
//   [Complaint description]
//   📍 Location, Area
//
// Features:
//   - HTML formatting for better readability
//   - Inline keyboard with "Mark as Resolved" button
//   - Returns message ID for future editing
//
// Parameters:
//   - complaintJSON: JSON string with complaint details
//   - complaintNumber: Complaint ID for callback data
//
// Returns:
//   - string: Telegram message ID
//   - error: Send error
func (c *Client) SendComplaintMessage(complaintJSON string, complaintNumber string) (string, error) {
	if c == nil {
		log.Println("   ⚠️  Telegram not configured, skipping message send")
		return "", nil
	}

	log.Println("   📨 Sending complaint to Telegram...")

	// Parse JSON to extract fields
	var complaint map[string]interface{}
	err := json.Unmarshal([]byte(complaintJSON), &complaint)
	if err != nil {
		return "", fmt.Errorf("failed to parse complaint JSON: %w", err)
	}

	// Helper function to safely extract values (handles null)
	getValue := func(key string) string {
		val := complaint[key]
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	}

	// Format message with emojis and structure
	message := fmt.Sprintf(
		"📋 Complaint : %s\n\n"+
			"👤 %s\n"+
			"📞 %s\n"+
			"🆔 Consumer: %s\n"+
			"📅 %s\n\n"+
			"💬 <b>Details:</b>\n%s\n\n"+
			"📍 %s, %s",
		getValue("complain_no"),
		getValue("complainant_name"),
		getValue("mobile_no"),
		getValue("consumer_no"),
		getValue("complain_date"),
		getValue("description"),
		getValue("exact_location"),
		getValue("area"),
	)

	// Create inline keyboard with "Mark as Resolved" button
	// Callback data format: "resolve:COMPLAINT_NUMBER"
	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{
					Text:         "✅ Mark as Resolved",
					CallbackData: fmt.Sprintf("resolve:%s", complaintNumber),
				},
			},
		},
	}

	telegramMsg := Message{
		ChatID:                c.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
		ReplyMarkup:           keyboard,
	}

	result, err := c.doRequest("sendMessage", telegramMsg)
	if err != nil {
		return "", fmt.Errorf("failed to send Telegram message: %w", err)
	}

	// Extract message ID from response
	var messageID string
	if msgResult, ok := result["result"].(map[string]interface{}); ok {
		if msgID, ok := msgResult["message_id"].(float64); ok {
			messageID = fmt.Sprintf("%.0f", msgID)
		}
	}

	log.Println("   ✓ Complaint successfully sent to Telegram")
	return messageID, nil
}

// SendCriticalAlert sends a critical failure alert to Telegram.
//
// This is called when all retry attempts fail and manual intervention is needed.
//
// Alert format:
//   🚨 CRITICAL ALERT - CMON SERVICE
//   Error Type: Fetch/Login Failure
//   Error Message: [details]
//   Retry Attempts: 3
//   Timestamp: 2026-01-15 10:30:00
//   ⚠️ Action Required: Please check the service immediately.
//
// Parameters:
//   - errorType: Type of error (e.g., "Fetch/Login Failure")
//   - errorMsg: Detailed error message
//   - retryCount: Number of retry attempts made
//
// Returns:
//   - error: Send error
func (c *Client) SendCriticalAlert(errorType, errorMsg string, retryCount int) error {
	if c == nil {
		log.Println("   ⚠️  Telegram not configured, skipping critical alert")
		return nil
	}

	log.Println("   🚨 Sending critical alert to Telegram...")

	message := fmt.Sprintf(
		"🚨 <b>CRITICAL ALERT - CMON SERVICE</b>\n\n"+
			"<b>Error Type:</b> %s\n"+
			"<b>Error Message:</b> %s\n"+
			"<b>Retry Attempts:</b> %d\n"+
			"<b>Timestamp:</b> %s\n\n"+
			"⚠️ <b>Action Required:</b> Please check the service immediately.",
		errorType,
		errorMsg,
		retryCount,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	telegramMsg := Message{
		ChatID:                c.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}

	_, err := c.doRequest("sendMessage", telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to send Telegram alert: %w", err)
	}

	log.Println("   ✓ Critical alert successfully sent to Telegram")
	return nil
}

// EditMessageText edits an existing Telegram message.
//
// Use cases:
//   - Marking complaint as resolved
//   - Updating complaint status
//
// Parameters:
//   - chatID: Chat ID where message is located
//   - messageID: Message ID to edit
//   - newText: New message text
//
// Returns:
//   - error: Edit error
func (c *Client) EditMessageText(chatID, messageID, newText string) error {
	if c == nil {
		log.Println("   ⚠️  Telegram not configured, skipping message edit")
		return nil
	}

	if messageID == "" {
		log.Println("   ⚠️  No message ID provided, skipping edit")
		return nil
	}

	log.Println("   📝 Editing Telegram message...")

	req := EditMessageRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      newText,
		ParseMode: "HTML",
	}

	_, err := c.doRequest("editMessageText", req)
	if err != nil {
		return fmt.Errorf("failed to edit Telegram message: %w", err)
	}

	log.Println("   ✓ Message successfully edited")
	return nil
}

// getUpdates fetches new updates from Telegram using long polling.
//
// Long polling:
//   - Keeps connection open for up to 30 seconds
//   - Returns immediately if updates are available
//   - Returns empty array if timeout with no updates
//
// Parameters:
//   - offset: Update ID to start from (for acknowledging processed updates)
//
// Returns:
//   - []Update: List of updates
//   - error: Request error
func (c *Client) getUpdates(offset int) ([]Update, error) {
	if c == nil {
		return nil, nil
	}

	payload := map[string]interface{}{
		"offset":  offset,
		"timeout": 30, // Long polling timeout in seconds
	}

	result, err := c.doRequest("getUpdates", payload)
	if err != nil {
		return nil, err
	}

	var updates []Update
	if resultArray, ok := result["result"].([]interface{}); ok {
		for _, item := range resultArray {
			jsonData, _ := json.Marshal(item)
			var update Update
			if err := json.Unmarshal(jsonData, &update); err == nil {
				updates = append(updates, update)
			}
		}
	}

	return updates, nil
}

// answerCallbackQuery sends a response to a callback query.
//
// This acknowledges the button click and optionally shows a notification.
//
// Parameters:
//   - callbackQueryID: ID of the callback query to answer
//   - text: Text to show in notification (optional)
//
// Returns:
//   - error: Request error
func (c *Client) answerCallbackQuery(callbackQueryID string, text string) error {
	if c == nil {
		return nil
	}

	payload := map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
		"show_alert":        false,
	}

	_, err := c.doRequest("answerCallbackQuery", payload)
	return err
}

// HandleUpdates listens for incoming updates and processes them.
//
// This runs in a background goroutine and handles:
//   - Callback queries (button clicks)
//   - Text messages (resolution notes)
//
// Update processing loop:
//   1. Long poll for updates (30s timeout)
//   2. Process each update
//   3. Update offset to acknowledge processed updates
//   4. Repeat until context is cancelled
//
// Parameters:
//   - ctx: Context for cancellation
//   - browserCtx: Browser context holder for API calls
//   - storage: Storage for complaint data
func (c *Client) HandleUpdates(ctx context.Context, browserCtx interface{}, storage *storage.Storage) {
	if c == nil {
		log.Println("⚠️  Telegram not configured, callback handler disabled")
		return
	}

	log.Println("✓ Starting Telegram callback handler...")
	offset := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Telegram callback handler stopped")
			return
		default:
			updates, err := c.getUpdates(offset)
			if err != nil {
				log.Printf("⚠️  Error getting Telegram updates: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			for _, update := range updates {
				if update.CallbackQuery != nil {
					c.handleCallbackQuery(ctx, update.CallbackQuery, storage)
				} else if update.Message != nil {
					c.handleMessage(ctx, browserCtx, update.Message, storage)
				}
				offset = update.UpdateID + 1
			}
		}
	}
}

// handleCallbackQuery processes a callback query from an inline button.
//
// Flow when user clicks "Mark as Resolved":
//   1. Parse callback data to get complaint number
//   2. Store pending resolution with complaint details
//   3. Send prompt message asking for resolution note
//   4. Wait for user's text message reply
//
// Parameters:
//   - ctx: Context for cancellation
//   - query: Callback query to process
//   - storage: Storage for complaint data
func (c *Client) handleCallbackQuery(ctx context.Context, query *CallbackQuery, storage *storage.Storage) {
	log.Printf("📞 Received callback query: %s from %s\n", query.Data, query.From.FirstName)

	// Parse callback data (format: "resolve:COMPLAINT_NUMBER")
	parts := strings.SplitN(query.Data, ":", 2)
	if len(parts) != 2 || parts[0] != "resolve" {
		log.Println("⚠️  Invalid callback data format")
		c.answerCallbackQuery(query.ID, "Invalid action")
		return
	}

	complaintNumber := parts[1]

	// Get message ID for this complaint
	messageID := storage.GetMessageID(complaintNumber)
	if messageID == "" {
		log.Println("⚠️  Message ID not found for complaint")
		c.answerCallbackQuery(query.ID, "Error: Message not found")
		return
	}

	// Get original message text
	originalText := ""
	if query.Message != nil {
		originalText = query.Message.Text
	}

	// Store pending resolution
	c.mu.Lock()
	// Check if resolution is already pending for this user and complaint (Toggle logic)
	if pending, exists := c.pendingResolutions[query.From.ID]; exists && pending.ComplaintNumber == complaintNumber {
		// User clicked button again -> CANCEL action
		delete(c.pendingResolutions, query.From.ID)
		c.mu.Unlock()

		// Delete the previous prompt message
		if pending.PromptMessageID > 0 {
			deleteReq := struct {
				ChatID    string `json:"chat_id"`
				MessageID int    `json:"message_id"`
			}{
				ChatID:    c.ChatID,
				MessageID: pending.PromptMessageID,
			}
			c.doRequest("deleteMessage", deleteReq)
		}

		c.answerCallbackQuery(query.ID, "Resolution cancelled")
		log.Printf("❌ Resolution cancelled by toggle for user %s\n", query.From.FirstName)
		return
	}

	c.pendingResolutions[query.From.ID] = PendingResolution{
		ComplaintNumber: complaintNumber,
		MessageID:       messageID,
		OriginalText:    originalText,
	}
	c.mu.Unlock()

	log.Printf("📝 Requesting resolution note for complaint %s from %s\n", complaintNumber, query.From.FirstName)

	// Extract consumer name from original text
	consumerName := "Unknown"
	if idx := strings.Index(originalText, "👤 "); idx != -1 {
		nameStart := idx + len("👤 ")
		if newlineIdx := strings.Index(originalText[nameStart:], "\n"); newlineIdx != -1 {
			consumerName = originalText[nameStart : nameStart+newlineIdx]
		}
	}

	// Send prompt message asking for resolution note
	originalMessageID, _ := strconv.Atoi(messageID)
	promptMsg := Message{
		ChatID:           c.ChatID,
		Text:             fmt.Sprintf("📝 Remarks for complaint <b>%s</b>\n👤 %s:", complaintNumber, consumerName),
		ParseMode:        "HTML",
		ReplyToMessageID: originalMessageID,
		ReplyMarkup: &ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: "Enter resolution details...",
		},
	}

	result, err := c.doRequest("sendMessage", promptMsg)
	if err != nil {
		log.Printf("⚠️  Failed to send prompt message: %v\n", err)
		c.answerCallbackQuery(query.ID, "Error sending prompt")
		return
	}

	// Extract prompt message ID for later deletion
	var promptMsgID int
	if msgResult, ok := result["result"].(map[string]interface{}); ok {
		if msgID, ok := msgResult["message_id"].(float64); ok {
			promptMsgID = int(msgID)
		}
	}

	// Update pending resolution with prompt message ID
	c.mu.Lock()
	if pending, exists := c.pendingResolutions[query.From.ID]; exists {
		pending.PromptMessageID = promptMsgID
		c.pendingResolutions[query.From.ID] = pending
	}
	c.mu.Unlock()

	c.answerCallbackQuery(query.ID, "Please send your remarks")
	log.Printf("✓ Prompted %s for remarks\n", query.From.FirstName)
}

// handleMessage processes regular text messages (for resolution notes).
//
// Flow when user sends resolution note:
//   1. Check if user has pending resolution
//   2. Delete prompt message (keep chat clean)
//   3. Call API to mark complaint as resolved on website
//   4. Edit original Telegram message to show "RESOLVED"
//   5. Remove complaint from storage
//
// Parameters:
//   - ctx: Context for cancellation
//   - browserCtx: Browser context for API calls
//   - message: Incoming message
//   - storage: Storage for complaint data
func (c *Client) handleMessage(ctx context.Context, browserCtx interface{}, message *IncomingMessage, storage *storage.Storage) {
	// Only process text messages from users with pending resolutions
	if message.From == nil || message.Text == "" {
		return
	}

	c.mu.Lock()
	pending, exists := c.pendingResolutions[message.From.ID]
	if !exists {
		c.mu.Unlock()
		return // No pending resolution for this user
	}

	promptMsgID := pending.PromptMessageID
	delete(c.pendingResolutions, message.From.ID)
	c.mu.Unlock()

	// Delete prompt message to keep chat clean
	if promptMsgID > 0 {
		deleteReq := struct {
			ChatID    string `json:"chat_id"`
			MessageID int    `json:"message_id"`
		}{
			ChatID:    c.ChatID,
			MessageID: promptMsgID,
		}
		c.doRequest("deleteMessage", deleteReq)
	}

	// Check for "cancel" keyword (Case-insensitive)
	if strings.EqualFold(strings.TrimSpace(message.Text), "cancel") {
		log.Printf("❌ Resolution cancelled by keyword for user %s\n", message.From.FirstName)
		msg := Message{
			ChatID:    c.ChatID,
			Text:      "❌ Resolution cancelled.",
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", msg)
		return
	}

	log.Printf("📝 Received resolution note from %s for complaint %s\n", message.From.FirstName, pending.ComplaintNumber)

	// Check if complaint still exists
	if !storage.Exists(pending.ComplaintNumber) {
		log.Printf("⚠️  Complaint %s was already resolved\n", pending.ComplaintNumber)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("ℹ️ Complaint <b>%s</b> was already resolved.", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	// Get API ID for resolution call
	apiID := storage.GetAPIID(pending.ComplaintNumber)
	if apiID == "" {
		log.Printf("⚠️  No API ID found for complaint %s\n", pending.ComplaintNumber)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Error: Cannot resolve complaint %s (API ID not found).", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	// Call API to mark complaint as resolved
	log.Printf("🌐 Calling DGVCL API to mark complaint %s as resolved...\n", pending.ComplaintNumber)

	// Extract browser context from interface
	var browserContext context.Context
	if holder, ok := browserCtx.(interface{ Get() context.Context }); ok {
		browserContext = holder.Get()
	} else {
		log.Printf("⚠️  Invalid browser context type\n")
		return
	}

	err := api.ResolveComplaint(browserContext, apiID, message.Text, c.DebugMode)
	if err != nil {
		log.Printf("⚠️  Failed to mark complaint on website: %v\n", err)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Failed to mark complaint %s as resolved on website: %v\nPlease try again or contact support.", pending.ComplaintNumber, err),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	log.Printf("✅ Successfully marked complaint %s as resolved on website\n", pending.ComplaintNumber)

	// Extract consumer name from original text
	consumerName := "Unknown"
	if idx := strings.Index(pending.OriginalText, "👤 "); idx != -1 {
		nameStart := idx + len("👤 ")
		if newlineIdx := strings.Index(pending.OriginalText[nameStart:], "\n"); newlineIdx != -1 {
			consumerName = pending.OriginalText[nameStart : nameStart+newlineIdx]
		}
	}

	// Create resolved message
	resolvedMessage := fmt.Sprintf(
		"✅ <b>RESOLVED</b>\n\n"+
			"Complaint #%s\n"+
			"👤 %s\n"+
			"🕐 %s",
		pending.ComplaintNumber,
		consumerName,
		time.Now().Format("02 Jan 2006, 03:04 PM"),
	)

	// Edit message and remove button
	req := EditMessageRequest{
		ChatID:      c.ChatID,
		MessageID:   pending.MessageID,
		Text:        resolvedMessage,
		ParseMode:   "HTML",
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}},
	}

	_, err = c.doRequest("editMessageText", req)
	if err != nil {
		log.Printf("⚠️  Failed to edit message: %v\n", err)
		errorMsg := Message{
			ChatID:    c.ChatID,
			Text:      fmt.Sprintf("❌ Error updating Telegram message for complaint %s. The complaint was marked as resolved on the website though.", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		c.doRequest("sendMessage", errorMsg)
		return
	}

	// Remove from storage
	removed, err := storage.RemoveIfExists(pending.ComplaintNumber)
	if err != nil {
		log.Printf("⚠️  Failed to remove from storage: %v\n", err)
	} else if !removed {
		log.Printf("ℹ️  Complaint %s was already removed from storage\n", pending.ComplaintNumber)
	}

	log.Printf("✓ Successfully resolved complaint %s with note\n", pending.ComplaintNumber)
}
