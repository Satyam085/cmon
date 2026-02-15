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

	// Health check server configuration
	HealthCheckPort string // Port for health check HTTP server

	// Debug mode - skips actual API calls for testing
	DebugMode bool

	// Google Cloud Translation (optional)
	GeminiAPIKey string // Gemini API key for Gujarati transliteration

	// Performance tuning (NEW)
	WorkerPoolSize int           // Number of concurrent workers for complaint processing
	CacheEnabled   bool          // Enable in-memory caching
	BatchSize      int           // Number of records to batch before writing to CSV
	HTTPMaxConns   int           // Maximum HTTP connections in pool
	HTTPTimeout    time.Duration // HTTP client timeout
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
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),

		// Health check - default port 8080
		HealthCheckPort: getEnvOrDefault("HEALTH_CHECK_PORT", "8080"),

		// Debug mode - default false (production mode)
		DebugMode: getEnvOrDefault("DEBUG_MODE", "false") == "true",

		// Google Cloud Translation (optional)
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),

		// Performance tuning (NEW) - optimized defaults
		WorkerPoolSize: getEnvInt("WORKER_POOL_SIZE", 10),                   // 10 concurrent workers
		CacheEnabled:   getEnvOrDefault("CACHE_ENABLED", "true") == "true",  // Cache enabled by default
		BatchSize:      getEnvInt("BATCH_SIZE", 50),                         // Batch 50 records before CSV write
		HTTPMaxConns:   getEnvInt("HTTP_MAX_CONNS", 100),                    // 100 connection pool size
		HTTPTimeout:    getEnvDuration("HTTP_TIMEOUT", 30*time.Second),      // 30s HTTP timeout
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
	if c.BatchSize < 1 {
		return fmt.Errorf("BATCH_SIZE must be at least 1, got %d", c.BatchSize)
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
