// Package session provides an authenticated HTTP client for the DGVCL portal.
//
// This package replaces the previous browser (ChromeDP) approach with a
// standard net/http client backed by a cookie jar. The DGVCL portal uses
// standard session cookies, so once we log in via HTTP the cookie jar
// automatically attaches the session cookie to every subsequent request.
//
// Key features:
//   - Thread-safe cookie jar shared across all goroutines
//   - Login with automatic arithmetic captcha solving
//   - Session expiry detection via HTML presence check
//   - Concurrency-safe Reset() for re-login / session recovery
package session

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"cmon/internal/errors"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
)

// Client is a thread-safe HTTP client that maintains an authenticated session
// with the DGVCL portal.
//
// The portal uses Laravel Sanctum — login returns a Bearer token which must
// be sent as `Authorization: Bearer <token>` on every subsequent request.
type Client struct {
	http        *http.Client
	mu          sync.RWMutex // protects bearerToken and baseURL
	baseURL     string       // root host, used for session expiry checks
	bearerToken string       // Sanctum Bearer token set after successful login
}

// New creates a new session client with a fresh, empty cookie jar.
//
// The underlying http.Client shares connection pools across all callers
// through the transport. The cookie jar is per-Client and is the key
// mechanism that stores the authenticated session.
func New() (*Client, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	transport := &http.Transport{
		// The DGVCL portal uses a certificate signed by a private CA that is
		// not in Go's system certificate store (Chrome had its own store and
		// trusted it). Skipping verification restores the same behaviour.
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}

	return &Client{
		http: &http.Client{
			Timeout:   30 * time.Second,
			Jar:       jar,
			Transport: transport,
		},
	}, nil
}

// Reset clears the bearer token and cookie jar, forcing a full re-login.
func (c *Client) Reset() error {
	c.mu.Lock()
	c.bearerToken = ""
	c.mu.Unlock()

	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return fmt.Errorf("failed to reset cookie jar: %w", err)
	}
	c.http.Jar = jar
	log.Println("  ✓ Session reset (token cleared)")
	return nil
}

// login authenticates with the DGVCL portal.
//
// The portal uses a JavaScript-driven login — the browser intercepts the form
// submit and POSTs JSON to /api/login rather than submitting the HTML form.
//
// Flow:
//  1. GET the login page → parse captcha + extract x-csrf-token from meta tag
//  2. Solve arithmetic captcha
//  3. POST JSON credentials to /api/login with X-CSRF-Token header
//  4. Verify session by checking dashboard is accessible (no login form)
func (c *Client) Login(loginURL, username, password string) error {
	// Remember base host for all subsequent requests
	if parsed, err := url.Parse(loginURL); err == nil {
		c.mu.Lock()
		c.baseURL = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		c.mu.Unlock()
	}

	// Step 1: GET the login page
	loginDoc, err := c.GetDoc(loginURL)
	if err != nil {
		return errors.NewLoginFailedError("failed to load login page", err)
	}
	// Step 2: Extract CSRF token — Laravel embeds it in <meta name="csrf-token">
	csrfToken := loginDoc.Find(`meta[name="csrf-token"]`).AttrOr("content", "")
	if csrfToken == "" {
		// Fallback: some pages put it in a hidden input named _token
		csrfToken = loginDoc.Find(`input[name="_token"]`).AttrOr("value", "")
	}
	if csrfToken == "" {
		log.Println("  ⚠️  No CSRF token found — proceeding without it")
	}

	// Step 3: Extract and solve captcha
	captchaText := strings.TrimSpace(loginDoc.Find("li.captchaList span").First().Text())
	if captchaText == "" {
		return errors.NewLoginFailedError("captcha text not found on login page", fmt.Errorf("selector li.captchaList span returned empty"))
	}
	captchaAnswer, err := solveCaptcha(captchaText)
	if err != nil {
		return errors.NewLoginFailedError("captcha solution failed", err)
	}

	// Step 4: POST JSON to /api/login
	// The browser JavaScript intercepts the form submit and sends JSON here.
	c.mu.RLock()
	apiLoginURL := c.baseURL + "/api/login"
	c.mu.RUnlock()

	payload := map[string]interface{}{
		"email_or_username": username,
		"password":          password,
		"captcha":           captchaAnswer,
		"complaint_source":  "",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.NewLoginFailedError("failed to marshal login payload", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiLoginURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return errors.NewLoginFailedError("failed to create login request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", loginURL)
	req.Header.Set("Origin", c.baseURL)
	if csrfToken != "" {
		req.Header.Set("X-CSRF-TOKEN", csrfToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.NewLoginFailedError("failed to submit login request", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.NewLoginFailedError(fmt.Sprintf("login API returned HTTP %d: %s", resp.StatusCode, string(respBody)), nil)
	}

	// Step 5: Extract Bearer token from JSON response
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(respBody, &loginResp); err != nil || loginResp.Token == "" {
		return errors.NewLoginFailedError("login API response missing token", err)
	}
	c.mu.Lock()
	c.bearerToken = loginResp.Token
	c.mu.Unlock()
	return nil
}

// IsSessionExpired checks whether the current session is still valid by
// fetching the dashboard root and checking if we get redirected to the
// login page (i.e., if the login form is present in the response HTML).
//
// Parameters:
//   - dashboardURL: URL of the authenticated area to probe
func (c *Client) IsSessionExpired(dashboardURL string) bool {
	doc, err := c.GetDoc(dashboardURL)
	if err != nil {
		// Network error — assume session might be expired to trigger retry
		return true
	}
	// Login form present → session expired
	return doc.Find("#email_or_username").Length() > 0
}

// GetDoc fetches a URL via GET and returns a parsed goquery Document.
// The cookie jar automatically sends any session cookies.
func (c *Client) GetDoc(rawURL string) (*goquery.Document, error) {
	resp, err := c.get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return goquery.NewDocumentFromReader(resp.Body)
}

// GetJSON fetches a URL via GET with XHR + Bearer auth headers.
func (c *Client) GetJSON(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json, text/javascript, */*")
	c.mu.RLock()
	token := c.bearerToken
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned HTTP %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, nil
}

// PostForm sends a POST request with URL-encoded form data, Bearer auth, and XHR header.
func (c *Client) PostForm(rawURL string, data url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.mu.RLock()
	token := c.bearerToken
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s failed: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s returned HTTP %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, nil
}

// get is a thin internal helper that does a plain GET and returns the response.
// Automatically adds the Bearer token if one has been set.
func (c *Client) get(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	c.mu.RLock()
	token := c.bearerToken
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s returned HTTP %d", rawURL, resp.StatusCode)
	}
	return resp, nil
}

// solveCaptcha solves the arithmetic captcha used on the DGVCL portal login page.
//
// Supports: +  (addition), -  (subtraction), × / x / * (multiplication)
// Input examples:  "5 + 3"  "12 - 4"  "3 × 7"
func solveCaptcha(text string) (string, error) {
	text = strings.TrimSpace(text)

	// Match: <number> <operator> <number>
	re := regexp.MustCompile(`(\d+)\s*([\+\-×xX\*])\s*(\d+)`)
	matches := re.FindStringSubmatch(text)

	var a, b int
	var op string

	if len(matches) == 4 {
		var err1, err2 error
		a, err1 = strconv.Atoi(matches[1])
		b, err2 = strconv.Atoi(matches[3])
		op = matches[2]
		if err1 != nil || err2 != nil {
			return "", fmt.Errorf("captcha parse failed (numbers) for %q: %v %v", text, err1, err2)
		}
	} else {
		// Fallback: whitespace-split
		parts := strings.Fields(text)
		if len(parts) < 3 {
			log.Printf("  ⚠️  Captcha raw text (parse failed): %q", text)
			return "", fmt.Errorf("invalid captcha format: %q", text)
		}
		var err1, err2 error
		a, err1 = strconv.Atoi(parts[0])
		b, err2 = strconv.Atoi(parts[2])
		op = parts[1]
		if err1 != nil || err2 != nil {
			log.Printf("  ⚠️  Captcha raw text (number parse failed): %q", text)
			return "", fmt.Errorf("invalid captcha numbers in %q", text)
		}
	}

	switch op {
	case "+":
		return strconv.Itoa(a + b), nil
	case "-":
		return strconv.Itoa(a - b), nil
	case "×", "x", "X", "*":
		return strconv.Itoa(a * b), nil
	default:
		log.Printf("  ⚠️  Unknown captcha operator %q in %q", op, text)
		return "", fmt.Errorf("unknown captcha operator %q in %q", op, text)
	}
}
