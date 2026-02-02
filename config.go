package main

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

//go:embed .env
var embeddedEnv string

// Config holds all application configuration
type Config struct {
	// URLs
	LoginURL     string
	ComplaintURL string
	
	// Authentication
	Username string
	Password string
	
	// Retry configuration
	MaxLoginRetries int
	LoginRetryDelay time.Duration
	MaxFetchRetries int
	
	// Pagination
	MaxPages int

	// Timing
	FetchInterval     time.Duration
	FetchTimeout      time.Duration
	NavigationTimeout time.Duration
	WaitTimeout       time.Duration
	
	// Telegram
	TelegramBotToken string
	TelegramChatID   string
	
	// Health check
	HealthCheckPort string
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() (*Config, error) {
	// Parse embedded env and set to env vars if not already set
	envMap, err := godotenv.Unmarshal(embeddedEnv)
	if err == nil {
		for k, v := range envMap {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	cfg := &Config{
		// URLs - could be overridden via env vars if needed
		LoginURL:     getEnvOrDefault("LOGIN_URL", "https://complaint.dgvcl.com/"),
		ComplaintURL: getEnvOrDefault("COMPLAINT_URL", "https://complaint.dgvcl.com/dashboard_complaint_list?from_date=&to_date=&honame=1&coname=21&doname=24&sdoname=87&cStatus=2&commobile="),
		
		// Authentication - required
		Username: os.Getenv("DGVCL_USERNAME"),
		Password: os.Getenv("DGVCL_PASSWORD"),
		
		// Retry configuration
		MaxLoginRetries: getEnvInt("MAX_LOGIN_RETRIES", 3),
		LoginRetryDelay: getEnvDuration("LOGIN_RETRY_DELAY", 5*time.Second),
		MaxFetchRetries: getEnvInt("MAX_FETCH_RETRIES", 2),
		
		// Pagination (Default 5 as requested)
		MaxPages: getEnvInt("MAX_PAGES", 5),

		// Timing
		FetchInterval:     getEnvDuration("FETCH_INTERVAL", 15*time.Minute),
		FetchTimeout:      getEnvDuration("FETCH_TIMEOUT", 10*time.Minute),
		NavigationTimeout: getEnvDuration("NAVIGATION_TIMEOUT", 30*time.Second),
		WaitTimeout:       getEnvDuration("WAIT_TIMEOUT", 20*time.Second),
		
		// Telegram
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		
		// Health check
		HealthCheckPort: getEnvOrDefault("HEALTH_CHECK_PORT", "8080"),
	}
	
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	
	return cfg, nil
}

// Validate checks that required configuration is present
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("DGVCL_USERNAME environment variable is required")
	}
	if c.Password == "" {
		return fmt.Errorf("DGVCL_PASSWORD environment variable is required")
	}
	if c.LoginURL == "" {
		return fmt.Errorf("LOGIN_URL cannot be empty")
	}
	if c.ComplaintURL == "" {
		return fmt.Errorf("COMPLAINT_URL cannot be empty")
	}
	return nil
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}