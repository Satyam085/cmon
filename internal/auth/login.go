// Package auth handles authentication and session management for the DGVCL portal.
//
// This package provides:
//   - Login automation with captcha solving
//   - Session expiry detection
//   - Retry logic with exponential backoff
//
// The DGVCL portal uses a simple arithmetic captcha for bot prevention.
// This package automatically solves the captcha during login.
package auth

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"cmon/internal/errors"

	"github.com/chromedp/chromedp"
)

// Login performs automated login to the DGVCL portal.
//
// Login flow:
//   1. Navigate to login page
//   2. Wait for page to load completely
//   3. Extract and solve arithmetic captcha
//   4. Fill in username, password, and captcha answer
//   5. Submit form and wait for redirect
//
// Captcha format:
//   - Simple arithmetic: "5 + 3" or "12 + 7"
//   - Displayed in <li class="captchaList"><span>5 + 3</span></li>
//   - Solution is the sum of the two numbers
//
// Error handling:
//   - Returns LoginFailedError for any step failure
//   - Caller should retry with delay or restart browser
//
// Parameters:
//   - ctx: Browser context for automation
//   - loginURL: URL of the login page
//   - username: DGVCL portal username
//   - password: DGVCL portal password
//
// Returns:
//   - error: LoginFailedError if login fails, nil on success
func Login(ctx context.Context, loginURL, username, password string) error {
	log.Println("  → Navigating to login page...")

	var captchaText string

	// Step 1-3: Navigate, wait for load, and extract captcha
	// All three operations are batched in a single chromedp.Run for efficiency
	err := chromedp.Run(ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible("body", chromedp.ByQuery), // Wait for page body
		chromedp.Text("li.captchaList span", &captchaText, chromedp.NodeVisible), // Extract captcha
	)
	if err != nil {
		log.Println("  ✗ Failed to load login page:", err)
		return errors.NewLoginFailedError("failed to load login page", err)
	}
	log.Println("  ✓ Login page loaded")

	// Step 4: Solve the arithmetic captcha
	log.Println("  → Solving captcha...")
	captchaAnswer, err := solveCaptcha(captchaText)
	if err != nil {
		log.Println("  ✗ Captcha error:", err)
		return errors.NewLoginFailedError("captcha solution failed", err)
	}
	log.Printf("  ✓ Captcha solved: %s = %s", captchaText, captchaAnswer)

	// Step 5: Fill form and submit
	log.Println("  → Submitting login credentials...")
	err = chromedp.Run(ctx,
		chromedp.SendKeys("#email_or_username", username), // Fill username field
		chromedp.SendKeys("#password", password),          // Fill password field
		chromedp.SendKeys("#captcha", captchaAnswer),      // Fill captcha answer
		chromedp.Click("button[type=submit]", chromedp.NodeVisible), // Click submit button
		chromedp.Sleep(3*time.Second), // Wait for redirect/processing
	)
	if err != nil {
		log.Println("  ✗ Failed to submit login form:", err)
		return errors.NewLoginFailedError("failed to submit login form", err)
	}

	log.Println("  ✓ Login successful")
	return nil
}

// IsSessionExpired checks if the current page indicates session expiration.
//
// Detection strategy:
//   - Check if login form is present (indicates redirect to login page)
//   - If login form exists, session has expired
//   - If login form doesn't exist, session is still valid
//
// This is more reliable than checking for missing dashboard elements,
// which can give false positives during page transitions.
//
// Use cases:
//   - After navigation errors
//   - Before retrying failed operations
//   - In error recovery logic
//
// Parameters:
//   - ctx: Browser context to check
//
// Returns:
//   - bool: true if session expired, false if still valid
func IsSessionExpired(ctx context.Context) bool {
	var loginFormExists bool

	// Check for the username input field (unique to login page)
	// Using JavaScript evaluation is faster than DOM queries
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("#email_or_username") !== null`, &loginFormExists),
	)

	// If we can check and the login form exists, session is expired
	if err == nil && loginFormExists {
		return true // Login form visible → Session expired
	}

	// Login form not found → Session still valid
	return false
}

// solveCaptcha solves the arithmetic captcha from the DGVCL portal.
//
// Captcha format:
//   - Input: "5 + 3" (string with two numbers and a plus sign)
//   - Output: "8" (string representation of the sum)
//
// Algorithm:
//   1. Split text by whitespace: ["5", "+", "3"]
//   2. Parse first number (index 0)
//   3. Parse second number (index 2)
//   4. Add them together
//   5. Convert result to string
//
// Error cases:
//   - Invalid format (not enough parts)
//   - Non-numeric values
//   - Missing operator (though we don't validate it's a plus)
//
// Parameters:
//   - text: Captcha text from the page (e.g., "5 + 3")
//
// Returns:
//   - string: Sum as a string (e.g., "8")
//   - error: Parsing error if format is invalid
func solveCaptcha(text string) (string, error) {
	// Split by whitespace: "5 + 3" → ["5", "+", "3"]
	parts := strings.Fields(text)

	// Validate format: need at least 3 parts (num, operator, num)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid captcha format: %q (expected format: 'X + Y')", text)
	}

	// Parse first number (index 0)
	a, err1 := strconv.Atoi(parts[0])
	// Parse second number (index 2, skipping operator at index 1)
	b, err2 := strconv.Atoi(parts[2])

	// Check if parsing succeeded
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("failed to parse captcha numbers: %q", text)
	}

	// Calculate sum and return as string
	return strconv.Itoa(a + b), nil
}
