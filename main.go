package main

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	// ===== CONFIG =====
	loginURL := "https://complaint.dgvcl.com/"

	username := "2124087_technical"
	password := "dgvcl1234"
	// ==================

	// Create browser context
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		chromedp.WithLogf(log.Printf),
	)
	defer cancel()

	// Timeout safety
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var captchaText string

	err := chromedp.Run(ctx,
		// Open login page
		chromedp.Navigate(loginURL),

		// Wait for page to load
		chromedp.WaitVisible("body", chromedp.ByQuery),

		// Read captcha text
		chromedp.Text(
			"li.captchaList span",
			&captchaText,
			chromedp.NodeVisible,
		),
	)
	if err != nil {
		log.Fatal("Failed to load page or read captcha:", err)
	}

	log.Println("Captcha text:", captchaText)

	// Solve captcha
	captchaAnswer := solveCaptcha(captchaText)
	log.Println("Captcha answer:", captchaAnswer)

	// Fill login form and submit
	err = chromedp.Run(ctx,
		chromedp.SendKeys("#email_or_username", username),
		chromedp.SendKeys("#password", password),
		chromedp.SendKeys("#captcha", captchaAnswer),
		chromedp.Click("button[type=submit]", chromedp.NodeVisible),
	)
	if err != nil {
		log.Fatal("Login submission failed:", err)
	}

	// ---- LOGIN SUCCESS CHECK ----
	// Change selector to something that only exists AFTER login
	err = chromedp.Run(ctx,
		chromedp.Sleep(3*time.Second),
	)

	if err != nil {
		log.Fatal("Error after login:", err)
	}

	log.Println("✅ Login successful")

	// Keep session alive for observation
	select {}
}

// ---------------- HELPERS ----------------

func solveCaptcha(text string) string {
	// Example: "6 + 17 ="
	parts := strings.Fields(text)

	if len(parts) < 3 {
		return ""
	}

	a, _ := strconv.Atoi(parts[0])
	op := parts[1]
	b, _ := strconv.Atoi(parts[2])

	result := 0
	switch op {
	case "+":
		result = a + b
	case "-":
		result = a - b
	case "*":
		result = a * b
	case "/":
		if b != 0 {
			result = a / b
		}
	}

	return strconv.Itoa(result)
}