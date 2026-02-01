package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
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
	
	// Parse JSON to extract fields
	var complaint map[string]interface{}
	err := json.Unmarshal([]byte(complaintJSON), &complaint)
	if err != nil {
		return fmt.Errorf("failed to parse complaint JSON: %w", err)
	}

	// Helper function to safely extract values, converting null to empty string
	getValue := func(key string) string {
		val := complaint[key]
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	}

	// Format the message in a user-friendly way
	message := fmt.Sprintf(
		"<b>--New Complaint--</b>\n\n" +
		"<b>Complaint Number:</b> %s\n" +
		"<b>Consumer Number:</b> %s\n" +
		"<b>Complainant Name:</b> %s\n" +
		"<b>Mobile Number:</b> %s\n" +
		"<b>Complaint Date:</b> %s\n\n" +
		"<b>Description:</b>\n%s\n\n" +
		"<b>Location:</b> %s\n" +
		"<b>Area:</b> %s",

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

	jsonData, err := json.Marshal(telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram alert: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tc.BotToken)
	
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send Telegram alert: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Telegram response: %w", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("failed to parse Telegram response: %w", err)
	}

	if ok, exists := result["ok"].(bool); !exists || !ok {
		return fmt.Errorf("Telegram API error: %v", result)
	}

	log.Println("   ✓ Critical alert successfully sent to Telegram")
	return nil
}

