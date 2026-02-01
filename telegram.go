package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

type TelegramMessage struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
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
	return &TelegramConfig{
		BotToken: botToken,
		ChatID:   chatID,
	}
}

// doTelegramRequest handles the common logic for sending requests to Telegram API
func (tc *TelegramConfig) doTelegramRequest(method string, payload interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", tc.BotToken, method)

	// Use shared HTTP client
	resp, err := GetHTTPClient().Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
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
		"<b>New Complaint</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━\n\n"+
			"📋 <b>Complaint No:</b> %s\n"+
			"🔢 <b>Consumer No:</b> %s\n\n"+
			"👤 <b>Complainant:</b> %s\n"+
			"📱 <b>Mobile:</b> %s\n"+
			"📅 <b>Date:</b> %s\n\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"<b>📝 Description:</b>\n<pre>%s</pre>\n\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📍 <b>Location:</b> %s\n"+
			"🗺️ <b>Area:</b> %s\n"+
			"━━━━━━━━━━━━━━━━━━━━\n",
		getValue("complain_no"),
		getValue("consumer_no"),
		getValue("complainant_name"),
		getValue("mobile_no"),
		getValue("complain_date"),
		getValue("description"),
		getValue("exact_location"),
		getValue("area"),
	)

	telegramMsg := TelegramMessage{
		ChatID:                tc.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
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
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
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
