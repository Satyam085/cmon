package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

type TelegramMessage struct {
	ChatID      string `json:"chat_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode"`
	DisableWebPagePreview bool `json:"disable_web_page_preview"`
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

func (tc *TelegramConfig) SendComplaintMessage(complaintJSON string, complaintNumber string) error {
	if tc == nil {
		log.Println("   ⚠️  Telegram not configured, skipping message send")
		return nil // Telegram not configured
	}

	log.Println("   📨 Sending complaint to Telegram...")
	// Format the message with complaint number as title
	message := fmt.Sprintf("🆕 <b>New Complaint: %s</b>\n\n<pre>%s</pre>", complaintNumber, complaintJSON)

	telegramMsg := TelegramMessage{
		ChatID:                tc.ChatID,
		Text:                  message,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}

	jsonData, err := json.Marshal(telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram message: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tc.BotToken)
	
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send Telegram message: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Telegram response: %w", err)
	}

	// Check if response is successful
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("failed to parse Telegram response: %w", err)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		return fmt.Errorf("Telegram API error: %v", result)
	}

	log.Println("   ✓ Complaint successfully sent to Telegram")
	return nil
}
