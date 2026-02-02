package main

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
)

// PendingResolution stores information about a complaint awaiting resolution note
type PendingResolution struct {
	ComplaintNumber  string
	MessageID        string
	OriginalText     string
	PromptMessageID  int    // ID of the prompt message to delete after user responds
}

type TelegramConfig struct {
	BotToken           string
	ChatID             string
	mu                 sync.Mutex
	pendingResolutions map[int64]PendingResolution // userID -> pending resolution
	DebugMode          bool                        // Skip API calls in debug mode
}

type TelegramMessage struct {
	ChatID                string      `json:"chat_id"`
	Text                  string      `json:"text"`
	ParseMode             string      `json:"parse_mode"`
	DisableWebPagePreview bool        `json:"disable_web_page_preview"`
	ReplyMarkup           interface{} `json:"reply_markup,omitempty"`
	ReplyToMessageID      int         `json:"reply_to_message_id,omitempty"` // For threading messages
}

// InlineKeyboardMarkup represents an inline keyboard
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton represents a button in an inline keyboard
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// ForceReply prompts user to reply to the bot's message
type ForceReply struct {
	ForceReply bool   `json:"force_reply"`
	Selective  bool   `json:"selective,omitempty"`
	InputFieldPlaceholder string `json:"input_field_placeholder,omitempty"`
}

// Update represents a Telegram update
type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// Message represents a Telegram message
type Message struct {
	MessageID int     `json:"message_id"`
	From      *User   `json:"from,omitempty"`
	Chat      *Chat   `json:"chat,omitempty"`
	Text      string  `json:"text"`
}

// Chat represents a Telegram chat
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// CallbackQuery represents a callback query from an inline button
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// User represents a Telegram user
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

func NewTelegramConfig() *TelegramConfig {
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
	
	// Get debug mode from environment
	debugMode := os.Getenv("DEBUG_MODE") == "true"
	if debugMode {
		log.Println("🐛 DEBUG MODE ENABLED - API calls will be simulated")
	}
	
	return &TelegramConfig{
		BotToken:           botToken,
		ChatID:             chatID,
		pendingResolutions: make(map[int64]PendingResolution),
		DebugMode:          debugMode,
	}
}

// doTelegramRequest handles the common logic for sending requests to Telegram API
func (tc *TelegramConfig) doTelegramRequest(method string, payload interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", tc.BotToken, method)

	// Use a custom HTTP client with longer timeout for Telegram API
	// This is necessary to support long polling (30s timeout)
	telegramClient := &http.Client{
		Timeout: 60 * time.Second, // Allow 60s to accommodate 30s long polling + network overhead
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

	if ok, exists := result["ok"].(bool); !exists || !ok {
		return nil, fmt.Errorf("Telegram API error: %v", result)
	}

	return result, nil
}

func (tc *TelegramConfig) SendComplaintMessage(complaintJSON string, complaintNumber string) (string, error) {
	if tc == nil {
		log.Println("   ⚠️  Telegram not configured, skipping message send")
		return "", nil // Telegram not configured
	}

	log.Println("   📨 Sending complaint to Telegram...")

	// Parse JSON to extract fields
	var complaint map[string]interface{}
	err := json.Unmarshal([]byte(complaintJSON), &complaint)
	if err != nil {
		return "", fmt.Errorf("failed to parse complaint JSON: %w", err)
	}

	// Helper function to safely extract values, converting null to empty string
	getValue := func(key string) string {
		val := complaint[key]
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	}

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

	telegramMsg := TelegramMessage{
		ChatID:                tc.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
		ReplyMarkup:           keyboard,
	}

	result, err := tc.doTelegramRequest("sendMessage", telegramMsg)
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

// SendCriticalAlert sends a critical failure alert to Telegram
func (tc *TelegramConfig) SendCriticalAlert(errorType, errorMsg string, retryCount int) error {
	if tc == nil {
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

	telegramMsg := TelegramMessage{
		ChatID:                tc.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}

	_, err := tc.doTelegramRequest("sendMessage", telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to send Telegram alert: %w", err)
	}

	log.Println("   ✓ Critical alert successfully sent to Telegram")
	return nil
}

type EditMessageRequest struct {
	ChatID      string                `json:"chat_id"`
	MessageID   string                `json:"message_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

func (tc *TelegramConfig) EditMessageText(chatID, messageID, newText string) error {
	if tc == nil {
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

	_, err := tc.doTelegramRequest("editMessageText", req)
	if err != nil {
		return fmt.Errorf("failed to edit Telegram message: %w", err)
	}

	log.Println("   ✓ Message successfully edited")
	return nil
}

// getUpdates fetches new updates from Telegram
func (tc *TelegramConfig) getUpdates(offset int) ([]Update, error) {
	if tc == nil {
		return nil, nil
	}

	payload := map[string]interface{}{
		"offset":  offset,
		"timeout": 30, // Long polling timeout
	}

	result, err := tc.doTelegramRequest("getUpdates", payload)
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

// answerCallbackQuery sends a response to a callback query
func (tc *TelegramConfig) answerCallbackQuery(callbackQueryID string, text string) error {
	if tc == nil {
		return nil
	}

	payload := map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
		"show_alert":        false,
	}

	_, err := tc.doTelegramRequest("answerCallbackQuery", payload)
	return err
}

// HandleUpdates listens for incoming updates and processes callback queries
func (tc *TelegramConfig) HandleUpdates(ctx context.Context, ctxHolder *BrowserContextHolder, storage *ComplaintStorage) {
	if tc == nil {
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
			updates, err := tc.getUpdates(offset)
			if err != nil {
				log.Printf("⚠️  Error getting Telegram updates: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			for _, update := range updates {
				if update.CallbackQuery != nil {
					tc.handleCallbackQuery(ctx, update.CallbackQuery, storage)
				} else if update.Message != nil {
					tc.handleMessage(ctx, ctxHolder.Get(), update.Message, storage)
				}
				offset = update.UpdateID + 1
			}
		}
	}
}

// handleCallbackQuery processes a callback query from an inline button
func (tc *TelegramConfig) handleCallbackQuery(ctx context.Context, query *CallbackQuery, storage *ComplaintStorage) {
	log.Printf("📞 Received callback query: %s from %s\n", query.Data, query.From.FirstName)

	// Parse callback data (format: "resolve:COMPLAINT_NUMBER")
	parts := strings.SplitN(query.Data, ":", 2)
	if len(parts) != 2 || parts[0] != "resolve" {
		log.Println("⚠️  Invalid callback data format")
		tc.answerCallbackQuery(query.ID, "Invalid action")
		return
	}

	complaintNumber := parts[1]

	// Get the message ID and original text for this complaint
	storage.mu.Lock()
	messageID := storage.messageIDs[complaintNumber]
	storage.mu.Unlock()

	if messageID == "" {
		log.Println("⚠️  Message ID not found for complaint")
		tc.answerCallbackQuery(query.ID, "Error: Message not found")
		return
	}

	// Get the original message text from the callback query
	originalText := ""
	if query.Message != nil {
		originalText = query.Message.Text
	}

	// Store pending resolution
	tc.mu.Lock()
	tc.pendingResolutions[query.From.ID] = PendingResolution{
		ComplaintNumber: complaintNumber,
		MessageID:       messageID,
		OriginalText:    originalText,
	}
	tc.mu.Unlock()

	log.Printf("📝 Requesting resolution note for complaint %s from %s\n", complaintNumber, query.From.FirstName)

	// Extract consumer name from original text
	consumerName := "Unknown"
	if idx := strings.Index(originalText, "👤 "); idx != -1 {
		nameStart := idx + len("👤 ")
		if newlineIdx := strings.Index(originalText[nameStart:], "\n"); newlineIdx != -1 {
			consumerName = originalText[nameStart : nameStart+newlineIdx]
		}
	}

	// Send message asking for resolution note with ForceReply
	// Reply to the original complaint message to keep conversation threaded
	originalMessageID, _ := strconv.Atoi(messageID)
	promptMsg := TelegramMessage{
		ChatID:           tc.ChatID,
		Text:             fmt.Sprintf("📝 Remarks for complaint <b>%s</b>\n👤 %s:", complaintNumber, consumerName),
		ParseMode:        "HTML",
		ReplyToMessageID: originalMessageID, // Thread reply to original complaint
		ReplyMarkup: &ForceReply{
			ForceReply:            true,
			InputFieldPlaceholder: "Enter resolution details...",
		},
	}

	// Send prompt message
	result, err := tc.doTelegramRequest("sendMessage", promptMsg)
	if err != nil {
		log.Printf("⚠️  Failed to send prompt message: %v\n", err)
		tc.answerCallbackQuery(query.ID, "Error sending prompt")
		return
	}
	
	// Extract actual prompt message ID from the response
	var promptMsgID int
	if msgResult, ok := result["result"].(map[string]interface{}); ok {
		if msgID, ok := msgResult["message_id"].(float64); ok {
			promptMsgID = int(msgID)
		}
	}
	
	// Update pending resolution with actual prompt message ID
	tc.mu.Lock()
	if pending, exists := tc.pendingResolutions[query.From.ID]; exists {
		pending.PromptMessageID = promptMsgID
		tc.pendingResolutions[query.From.ID] = pending
	}
	tc.mu.Unlock()

	// Answer the callback query
	tc.answerCallbackQuery(query.ID, "Please send your remarks")
	log.Printf("✓ Prompted %s for remarks\n", query.From.FirstName)
}

// handleMessage processes regular text messages (for resolution notes)
func (tc *TelegramConfig) handleMessage(ctx context.Context, browserCtx context.Context, message *Message, storage *ComplaintStorage) {
	// Only process text messages from users with pending resolutions
	if message.From == nil || message.Text == "" {
		return
	}

	tc.mu.Lock()
	pending, exists := tc.pendingResolutions[message.From.ID]
	if !exists {
		tc.mu.Unlock()
		return // No pending resolution for this user
	}
	
	// Store prompt message ID before removing from pending
	promptMsgID := pending.PromptMessageID
	
	// Remove from pending immediately
	delete(tc.pendingResolutions, message.From.ID)
	tc.mu.Unlock()

	// Delete the prompt message to keep chat clean
	if promptMsgID > 0 {
		deleteReq := struct {
			ChatID    string `json:"chat_id"`
			MessageID int    `json:"message_id"`
		}{
			ChatID:    tc.ChatID,
			MessageID: promptMsgID,
		}
		tc.doTelegramRequest("deleteMessage", deleteReq)
		// Ignore errors - message might already be deleted
	}

	log.Printf("📝 Received resolution note from %s for complaint %s\n", message.From.FirstName, pending.ComplaintNumber)

	// Check if complaint still exists in storage (may have been auto-resolved)
	if !storage.ExistsInStorage(pending.ComplaintNumber) {
		log.Printf("⚠️  Complaint %s was already resolved (possibly auto-resolved from website)\n", pending.ComplaintNumber)
		errorMsg := TelegramMessage{
			ChatID:    tc.ChatID,
			Text:      fmt.Sprintf("ℹ️ Complaint <b>%s</b> was already resolved.", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		tc.doTelegramRequest("sendMessage", errorMsg)
		return
	}

	// Get API ID for this complaint
	apiID := storage.GetAPIID(pending.ComplaintNumber)
	if apiID == "" {
		log.Printf("⚠️  No API ID found for complaint %s\n", pending.ComplaintNumber)
		errorMsg := TelegramMessage{
			ChatID:    tc.ChatID,
			Text:      fmt.Sprintf("❌ Error: Cannot resolve complaint %s (API ID not found).", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		tc.doTelegramRequest("sendMessage", errorMsg)
		return
	}

	// Call the API to mark complaint as resolved on the website
	log.Printf("🌐 Calling DGVCL API to mark complaint %s as resolved...\n", pending.ComplaintNumber)
	err := ResolveComplaintOnWebsite(browserCtx, apiID, message.Text, tc.DebugMode)
	if err != nil {
		log.Printf("⚠️  Failed to mark complaint on website: %v\n", err)
		errorMsg := TelegramMessage{
			ChatID:    tc.ChatID,
			Text:      fmt.Sprintf("❌ Failed to mark complaint %s as resolved on website: %v\nPlease try again or contact support.", pending.ComplaintNumber, err),
			ParseMode: "HTML",
		}
		tc.doTelegramRequest("sendMessage", errorMsg)
		return
	}

	log.Printf("✅ Successfully marked complaint %s as resolved on website\n", pending.ComplaintNumber)

	// Extract consumer name from original text (format: "👤 Name\n")
	consumerName := "Unknown"
	if idx := strings.Index(pending.OriginalText, "👤 "); idx != -1 {
		nameStart := idx + len("👤 ")
		if newlineIdx := strings.Index(pending.OriginalText[nameStart:], "\n"); newlineIdx != -1 {
			consumerName = pending.OriginalText[nameStart : nameStart+newlineIdx]
		}
	}

	// Create minimal resolved message - complaint number, consumer name, and resolved status
	resolvedMessage := fmt.Sprintf(
		"✅ <b>RESOLVED</b>\n\n"+
			"Complaint #%s\n"+
			"👤 %s\n"+
			"🕐 %s",
		pending.ComplaintNumber,
		consumerName,
		time.Now().Format("02 Jan 2006, 03:04 PM"),
	)

	// Remove the button when marking as resolved
	req := EditMessageRequest{
		ChatID:      tc.ChatID,
		MessageID:   pending.MessageID,
		Text:        resolvedMessage,
		ParseMode:   "HTML",
		ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}, // Empty keyboard
	}

	_, err = tc.doTelegramRequest("editMessageText", req)
	if err != nil {
		log.Printf("⚠️  Failed to edit message: %v\n", err)
		// Send error message to user
		errorMsg := TelegramMessage{
			ChatID:    tc.ChatID,
			Text:      fmt.Sprintf("❌ Error updating Telegram message for complaint %s. The complaint was marked as resolved on the website though.", pending.ComplaintNumber),
			ParseMode: "HTML",
		}
		tc.doTelegramRequest("sendMessage", errorMsg)
		return
	}

	// Remove from storage and CSV atomically
	removed, err := storage.RemoveIfExists(pending.ComplaintNumber)
	if err != nil {
		log.Printf("⚠️  Failed to remove from storage: %v\n", err)
	} else if !removed {
		log.Printf("ℹ️  Complaint %s was already removed from storage (concurrent resolution)\n", pending.ComplaintNumber)
	}

	log.Printf("✓ Successfully resolved complaint %s with note\n", pending.ComplaintNumber)
}
