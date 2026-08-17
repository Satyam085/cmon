package sfms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Client handles interaction with the GETCO SFMS REST API.
type Client struct {
	httpClient *http.Client
	config     *Config
	mu         sync.RWMutex
}

// NewClient initializes a new SFMS API Client.
func NewClient(cfg *Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
	}
}

func setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:153.0) Gecko/20100101 Firefox/153.0")
	req.Header.Set("Origin", "https://getco-sfms.in")
	req.Header.Set("Referer", "https://getco-sfms.in/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
}

// SetToken updates the client's Bearer token in memory.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.BearerToken = NormalizeBearerToken(token)
}

// GetToken returns the current Bearer token.
func (c *Client) GetToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.BearerToken
}

// Login authenticates against the GETCO SFMS login endpoint to obtain a new Bearer Token.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.Username == "" || c.config.Password == "" {
		return fmt.Errorf("cannot auto-login: username or password not configured")
	}

	payload := LoginRequest{
		Username: c.config.Username,
		Password: c.config.Password,
		CsComp:   "sfms",
		OTP:      "",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.LoginURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create login HTTP request: %w", err)
	}
	setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to decode login API JSON response: %w", err)
	}

	if loginResp.AccessToken == "" {
		return fmt.Errorf("login response did not contain access_token (result message: %s)", loginResp.Result.Message)
	}

	tokenStr := NormalizeBearerToken(loginResp.AccessToken)
	c.config.BearerToken = tokenStr

	// Persist new token to token.txt
	_ = os.WriteFile("token.txt", []byte(tokenStr), 0644)

	log.Printf("[%s] 🔑 [AUTH SUCCESS] Automatically obtained fresh Bearer Token for user '%s'",
		FormatNowIST(), c.config.Username)

	return nil
}

func (c *Client) reloadTokenIfChanged() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	tokenData, err := os.ReadFile("token.txt")
	if err != nil {
		tokenData, err = os.ReadFile("smfs/token.txt")
		if err != nil {
			return false
		}
	}
	fileToken := NormalizeBearerToken(string(tokenData))
	if fileToken != "" && fileToken != c.config.BearerToken {
		c.config.BearerToken = fileToken
		log.Printf("[%s] 🔑 [TOKEN RELOAD] Detected updated Bearer Token in token.txt. Applied immediately.",
			FormatNowIST())
		return true
	}
	return false
}

// FetchSubstations queries the GETCO SFMS REST API for the full list of substations and feeders.
func (c *Client) FetchSubstations(ctx context.Context) ([]Substation, error) {
	c.reloadTokenIfChanged()

	c.mu.RLock()
	token := c.config.BearerToken
	apiURL := c.config.APIURL
	depID := c.config.DepID
	divID := c.config.DivID
	c.mu.RUnlock()

	if token == "" {
		return nil, fmt.Errorf("bearer token is empty (please provide in token.txt, env, or dashboard)")
	}

	payload := RequestPayload{
		Depid: depID,
		DivID: divID,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	setCommonHeaders(req)
	req.Header.Set("Authorization", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d Unauthorized - Bearer token is expired or invalidated", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	if !apiResp.Result.Flag && len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("API error result: %s", apiResp.Result.Message)
	}

	return apiResp.Data, nil
}

// FetchDeviceMappedGroups queries GETCO SFMS API for device telemetry mapped group IDs.
func (c *Client) FetchDeviceMappedGroups(ctx context.Context, keyword string, deviceIDs []string) ([]DeviceGroup, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	c.reloadTokenIfChanged()

	c.mu.RLock()
	token := c.config.BearerToken
	c.mu.RUnlock()

	if token == "" {
		return nil, fmt.Errorf("bearer token is empty")
	}

	reqURL := fmt.Sprintf("https://api.getco-sfms.in/api/Device/GetDevicemappedGroup?GroupKeyWord_cs=%s&deviceId_cs=%s&PROJCD=sfms",
		keyword, strings.Join(deviceIDs, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeviceGroup HTTP request: %w", err)
	}

	setCommonHeaders(req)
	req.Header.Set("Authorization", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeviceGroup HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DeviceGroup API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var groupResp DeviceGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&groupResp); err != nil {
		return nil, fmt.Errorf("failed to decode DeviceGroup JSON response: %w", err)
	}

	if !groupResp.Result.Flag && len(groupResp.Data) == 0 {
		return nil, fmt.Errorf("DeviceGroup API error result: %s", groupResp.Result.Message)
	}

	return groupResp.Data, nil
}

// VerifyToken tests whether a Bearer token is valid against the GETCO SFMS API.
// Returns the number of substations loaded, or an error if invalid.
func (c *Client) VerifyToken(ctx context.Context, testToken string) (int, error) {
	norm := NormalizeBearerToken(testToken)
	if norm == "" {
		return 0, fmt.Errorf("token cannot be empty")
	}

	c.mu.RLock()
	apiURL := c.config.APIURL
	depID := c.config.DepID
	divID := c.config.DivID
	c.mu.RUnlock()

	payload := RequestPayload{
		Depid: depID,
		DivID: divID,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, fmt.Errorf("create request failed: %w", err)
	}
	setCommonHeaders(req)
	req.Header.Set("Authorization", norm)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, fmt.Errorf("token unauthorized or expired (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(b))
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return 0, fmt.Errorf("invalid API response: %w", err)
	}

	return len(apiResp.Data), nil
}
