package main

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func Login(ctx context.Context, loginURL, username, password string) error {
	log.Println("  → Navigating to login page...")
	var captchaText string

	err := chromedp.Run(ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Text("li.captchaList span", &captchaText, chromedp.NodeVisible),
	)
	if err != nil {
		log.Println("  ✗ Failed to load login page:", err)
		return NewLoginFailedError("failed to load login page", err)
	}
	log.Println("  ✓ Login page loaded")

	log.Println("  → Solving captcha...")
	captchaAnswer := solveCaptcha(captchaText)
	log.Println("  ✓ Captcha solved:", captchaAnswer)

	log.Println("  → Submitting login credentials...")
	err = chromedp.Run(ctx,
		chromedp.SendKeys("#email_or_username", username),
		chromedp.SendKeys("#password", password),
		chromedp.SendKeys("#captcha", captchaAnswer),
		chromedp.Click("button[type=submit]", chromedp.NodeVisible),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		log.Println("  ✗ Failed to submit login form:", err)
		return NewLoginFailedError("failed to submit login form", err)
	}

	log.Println("  ✓ Login successful")
	return nil
}

// IsSessionExpired checks if the current page indicates session expiration
// by verifying if the complaints table is visible
func IsSessionExpired(ctx context.Context) bool {
	var tableExists bool
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("table") !== null`, &tableExists),
	)
	
	// If there's an error checking or table doesn't exist, consider session expired
	if err != nil || !tableExists {
		return true
	}
	return false
}

func solveCaptcha(text string) string {
	parts := strings.Fields(text)
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[2])

	return strconv.Itoa(a + b)
}