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
		return err
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
		return err
	}

	log.Println("  ✓ Login successful")
	return nil
}

func solveCaptcha(text string) string {
	parts := strings.Fields(text)
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[2])

	return strconv.Itoa(a + b)
}