// Package config provides configuration management for the CMON application.
//
// This package handles loading configuration from environment variables,
// validating required settings, and providing sensible defaults for optional
// parameters. Configuration is loaded once at startup and remains immutable
// during runtime for thread-safety.
//
// Configuration sources (in order of precedence):
//  1. Environment variables (highest priority)
//  2. Embedded .env file (fallback, included in binary)
//  3. Hard-coded defaults (lowest priority)
package config

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// embeddedEnv contains the .env file embedded at build time.
//
// This allows the binary to work standalone without requiring
// an external .env file. The embedded file serves as a fallback
// when environment variables are not set.
//
// Security note: The embedded .env should contain template values
// or be overridden by environment variables in production.
//
//go:embed .env
var embeddedEnv string

// Config holds all application configuration.
//
// This struct is immutable after creation to ensure thread-safety.
// All duration and timeout values are configurable via environment
// variables to allow tuning for different network conditions.
type Config struct {
	// URLs for the DGVCL portal
	LoginURL     string // Login page URL
	ComplaintURL string // Dashboard URL with filters applied
	ResolveURL   string // POST endpoint that marks a complaint as resolved

	// Authentication credentials (required)
	Username string // DGVCL portal username
	Password string // DGVCL portal password

	// Retry configuration for resilience
	MaxLoginRetries int           // Maximum login attempts before giving up
	LoginRetryDelay time.Duration // Delay between login retry attempts
	MaxFetchRetries int           // Maximum fetch attempts before alerting

	// Pagination limits to prevent infinite loops
	MaxPages int // Maximum number of pages to fetch per cycle

	// Timing configuration for different operations
	FetchInterval     time.Duration // How often to check for new complaints
	FetchTimeout      time.Duration // Maximum time for entire fetch operation
	NavigationTimeout time.Duration // Maximum time for page navigation
	WaitTimeout       time.Duration // Maximum time to wait for elements

	// Telegram configuration (optional)
	TelegramBotToken string // Telegram bot API token
	TelegramChatID   string // Telegram chat ID for notifications

	// TelegramBeltRoutes maps a canonical belt key to a Telegram chat ID
	// override. Complaints whose belt is in the map are routed to the
	// matching chat instead of TelegramChatID. Empty disables routing.
	// Parsed from TELEGRAM_BELT_ROUTES env, format: "belt=chatID,belt=chatID".
	TelegramBeltRoutes map[string]string

	// WhatsApp configuration (optional)
	WhatsAppRecipientJID  string // Target JID, e.g. 919876543210@s.whatsapp.net
	WhatsAppDBPath        string // Path to SQLite session DB (default: whatsapp.db)
	WhatsAppResolveEnabled bool   // Allow resolve-by-reply from WhatsApp (default false)

	// Health check server configuration
	HealthCheckPort string // Port for health check HTTP server

	// LogFormat selects the structured logger output: "text" (terminal-friendly
	// logfmt-style) or "json" (parseable by log aggregators). Defaults to "text".
	LogFormat string

	// ScheduledSummaries is a list of HH:MM (IST) times at which the daemon
	// will auto-post a /summary cycle to Telegram + WhatsApp. Empty disables
	// the feature. Parsed in LoadConfig from a comma-separated env value
	// like "09:00,18:00".
	ScheduledSummaries []string

	// Debug mode - skips actual API calls for testing
	DebugMode bool

	// Google Cloud Translation (optional)
	GeminiAPIKey string // Gemini API key for Gujarati transliteration

	// Performance tuning
	WorkerPoolSize int           // Number of concurrent workers for complaint processing
	HTTPMaxConns   int           // Maximum HTTP connections in pool
	HTTPTimeout    time.Duration // HTTP client timeout

	// API rate limiting (DGVCL upstream returns 429 if we burst too fast)
	APIRateLimitRPS   float64 // Sustained req/s ceiling for the DGVCL API
	APIRateLimitBurst int     // Token-bucket burst size
	APIMaxRetries429  int     // Max 429 retry attempts per request
}

// LoadConfig loads configuration from environment variables with defaults.
//
// Loading process:
//   1. Parse embedded .env file and set as fallback environment variables
//   2. Try to load external .env file (overrides embedded values)
//   3. Read environment variables (highest priority, overrides all)
//   4. Apply hard-coded defaults for any missing optional values
//   5. Validate that all required fields are present
//
// This three-tier approach allows:
//   - Binary to work standalone (embedded .env)
//   - External .env to override embedded values
//   - Environment variables to override everything
//
// Returns:
//   - *Config: Fully populated configuration struct
//   - error: Validation error if required fields are missing
func LoadConfig() (*Config, error) {
	// Step 1: Parse embedded .env file and set as fallback
	// This allows the binary to work standalone without external .env file
	envMap, err := godotenv.Unmarshal(embeddedEnv)
	if err == nil {
		for k, v := range envMap {
			// Only set if not already in environment (env vars take precedence)
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	// Step 2: Try to load external .env file (optional, overrides embedded)
	// This allows updating config without rebuilding the binary
	_ = godotenv.Load()

	// Step 2 & 3: Build config from environment with defaults
	cfg := &Config{
		// URLs - can be overridden via env vars if portal URLs change
		LoginURL:     getEnvOrDefault("LOGIN_URL", "https://complaint.dgvcl.com/"),
		ComplaintURL: getEnvOrDefault("COMPLAINT_URL", "https://complaint.dgvcl.com/dashboard_complaint_list?from_date=&to_date=&honame=1&coname=21&doname=24&sdoname=87&cStatus=2&commobile="),
		ResolveURL:   getEnvOrDefault("DGVCL_RESOLVE_URL", "https://complaint.dgvcl.com/api/complaint-assign-process"),

		// Authentication - REQUIRED, no defaults
		Username: os.Getenv("DGVCL_USERNAME"),
		Password: os.Getenv("DGVCL_PASSWORD"),

		// Retry configuration - tuned for typical network conditions
		MaxLoginRetries: getEnvInt("MAX_LOGIN_RETRIES", 3),      // 3 attempts is usually enough
		LoginRetryDelay: getEnvDuration("LOGIN_RETRY_DELAY", 5*time.Second), // 5s between retries
		MaxFetchRetries: getEnvInt("MAX_FETCH_RETRIES", 2),      // 2 retries for fetch operations

		// Pagination - default 5 pages to balance coverage vs speed
		MaxPages: getEnvInt("MAX_PAGES", 5),

		// Timing - tuned for typical portal response times
		FetchInterval:     getEnvDuration("FETCH_INTERVAL", 15*time.Minute),     // Check every 15 minutes
		FetchTimeout:      getEnvDuration("FETCH_TIMEOUT", 10*time.Minute),      // 10 min total fetch timeout
		NavigationTimeout: getEnvDuration("NAVIGATION_TIMEOUT", 60*time.Second), // 60s for page loads
		WaitTimeout:       getEnvDuration("WAIT_TIMEOUT", 45*time.Second),       // 45s for element waits

		// Telegram - optional, notifications disabled if not set
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:     os.Getenv("TELEGRAM_CHAT_ID"),
		TelegramBeltRoutes: parseBeltRoutes(os.Getenv("TELEGRAM_BELT_ROUTES")),

		// WhatsApp - optional, notifications disabled if not set.
		// Resolve-by-reply defaults to true now that the flow is fully
		// scaffolded; set WHATSAPP_RESOLVE_ENABLED=false to disable.
		WhatsAppRecipientJID:   os.Getenv("WHATSAPP_RECIPIENT_JID"),
		WhatsAppDBPath:         getEnvOrDefault("WHATSAPP_DB_PATH", "whatsapp.db"),
		WhatsAppResolveEnabled: getEnvOrDefault("WHATSAPP_RESOLVE_ENABLED", "true") == "true",

		// Health check - default port 8080
		HealthCheckPort: getEnvOrDefault("HEALTH_CHECK_PORT", "8080"),

		// Log format - default text mode for terminal use
		LogFormat: getEnvOrDefault("LOG_FORMAT", "text"),

		// Scheduled summaries - empty by default (feature opt-in).
		ScheduledSummaries: parseScheduleList(os.Getenv("SCHEDULED_SUMMARIES")),

		// Debug mode - default false (production mode)
		DebugMode: getEnvOrDefault("DEBUG_MODE", "false") == "true",

		// Google Cloud Translation (optional)
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),

		// Performance tuning - optimized defaults
		WorkerPoolSize: getEnvInt("WORKER_POOL_SIZE", 10),      // 10 concurrent workers
		HTTPMaxConns:   getEnvInt("HTTP_MAX_CONNS", 100),       // 100 connection pool size
		HTTPTimeout:    getEnvDuration("HTTP_TIMEOUT", 30*time.Second), // 30s HTTP timeout

		// API rate limiting - keeps us under the DGVCL portal's 429 threshold
		APIRateLimitRPS:   getEnvFloat("API_RATE_LIMIT_RPS", 3.0),
		APIRateLimitBurst: getEnvInt("API_RATE_LIMIT_BURST", 5),
		APIMaxRetries429:  getEnvInt("API_MAX_RETRIES_429", 5),
	}

	// Step 4: Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that required configuration is present and values are sensible.
//
// Validation rules:
//   - Username and Password must be non-empty (required for login)
//   - URLs must be non-empty (required for navigation)
//   - Numeric values must be positive (negative values don't make sense)
//
// Returns:
//   - error: Descriptive error if validation fails, nil if all checks pass
func (c *Config) Validate() error {
	// Check required authentication credentials
	if c.Username == "" {
		return fmt.Errorf("DGVCL_USERNAME environment variable is required")
	}
	if c.Password == "" {
		return fmt.Errorf("DGVCL_PASSWORD environment variable is required")
	}

	// Check required URLs
	if c.LoginURL == "" {
		return fmt.Errorf("LOGIN_URL cannot be empty")
	}
	if c.ComplaintURL == "" {
		return fmt.Errorf("COMPLAINT_URL cannot be empty")
	}

	// Validate numeric values are positive
	if c.MaxPages < 1 {
		return fmt.Errorf("MAX_PAGES must be at least 1, got %d", c.MaxPages)
	}
	if c.WorkerPoolSize < 1 {
		return fmt.Errorf("WORKER_POOL_SIZE must be at least 1, got %d", c.WorkerPoolSize)
	}

	return nil
}

// Helper functions for environment variable parsing

// getEnvOrDefault returns the environment variable value or a default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns the environment variable as an integer or a default if not set/invalid
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvFloat returns the environment variable as a float64 or a default if not set/invalid
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// parseBeltRoutes turns "dahod=-1001234, bajipura=-1005678" into a map
// keyed by lowercase belt name. Tokens that don't fit the
// "<key>=<chat_id>" shape are dropped silently — strict parsing keeps the
// scheduler from sending to a half-typed chat ID. Empty input → nil.
//
// Belt keys are lowercased for case-insensitive matching against canonical
// belt names returned by belt.Resolve, but the chat ID is stored verbatim
// because Telegram chat IDs include a leading "-" (group/channel marker).
func parseBeltRoutes(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		eq := strings.IndexByte(tok, '=')
		if eq <= 0 || eq == len(tok)-1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(tok[:eq]))
		chatID := strings.TrimSpace(tok[eq+1:])
		if key == "" || chatID == "" {
			continue
		}
		out[key] = chatID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseScheduleList turns "09:00, 18:00" into ["09:00", "18:00"]. Tokens
// that don't match HH:MM (24-hour) are dropped — strict parsing keeps the
// scheduler from firing at surprising times if someone fat-fingers an entry.
// An empty input yields a nil slice.
func parseScheduleList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if !validHHMM(tok) {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// validHHMM checks the 24-hour HH:MM format. Strictly two digits for both
// fields so "9:5" doesn't smuggle in an off-by-an-hour misinterpretation.
func validHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	hh, err := strconv.Atoi(s[:2])
	if err != nil || hh < 0 || hh > 23 {
		return false
	}
	mm, err := strconv.Atoi(s[3:])
	if err != nil || mm < 0 || mm > 59 {
		return false
	}
	return true
}

// getEnvDuration returns the environment variable as a duration or a default if not set/invalid.
//
// Accepts standard Go duration strings like "5s", "10m", "1h30m"
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
