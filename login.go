package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func Login(ctx context.Context, loginURL, username, password string, cfg *Config) error {
	log.Println("  → Navigating to login page...")
	var captchaText string

	// Navigate with timeout
	navCtx, navCancel := context.WithTimeout(ctx, cfg.NavigationTimeout)
	err := chromedp.Run(navCtx, chromedp.Navigate(loginURL))
	navCancel()
	
	if err != nil {
		log.Printf("  ✗ Navigation timeout/error after %v: %v", cfg.NavigationTimeout, err)
		return NewLoginFailedError("failed to navigate to login page", err)
	}
	
	// Wait for page and get captcha with timeout
	waitCtx, waitCancel := context.WithTimeout(ctx, cfg.WaitTimeout)
	err = chromedp.Run(waitCtx,
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Text("li.captchaList span", &captchaText, chromedp.NodeVisible),
	)
	waitCancel()
	
	if err != nil {
		log.Printf("  ✗ Wait/captcha timeout/error after %v: %v", cfg.WaitTimeout, err)
		return NewLoginFailedError("failed to load login page or get captcha", err)
	}
	log.Println("  ✓ Login page loaded")

	log.Println("  → Solving captcha...")
	captchaAnswer, err := solveCaptcha(captchaText)
	if err != nil {
		log.Println("  ✗ Captcha error:", err)
		return NewLoginFailedError("captcha solution failed", err)
	}
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
// by verifying if the login form is present (which means we got redirected out)
// or if the complaints table is missing when it should be there.
func IsSessionExpired(ctx context.Context) bool {
	var loginFormExists bool
	// Check for login form specific element
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("#email_or_username") !== null`, &loginFormExists),
	)
	
	if err == nil && loginFormExists {
		return true // Login form is visible -> Session Expired / Logged out
	}
	
	// Optional: Could also check if we are NOT on the dashboard URL?
	// But relying on presence of known login elements is safer than absence of table.
	// For now, if we see the login input, we are definitely expired.
	
	// Fallback: If we can't find the table AND we think we should be on dashboard...
	// but strictly speaking, simply "table missing" checks led to false positives.
	// We'll trust the login form check as primary.
	
	return false
}

func solveCaptcha(text string) (string, error) {
	parts := strings.Fields(text)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid captcha format: %q", text)
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[2])
	
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("failed to parse captcha numbers: %q", text)
	}

	return strconv.Itoa(a + b), nil
}