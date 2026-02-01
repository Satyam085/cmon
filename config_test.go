package main

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Save original env vars
	origUsername := os.Getenv("DGVCL_USERNAME")
	origPassword := os.Getenv("DGVCL_PASSWORD")
	
	// Save original embeddedEnv and clear it
	origEmbeddedEnv := embeddedEnv
	embeddedEnv = ""
	
	// Clean up after test
	defer func() {
		if origUsername != "" {
			os.Setenv("DGVCL_USERNAME", origUsername)
		}
		if origPassword != "" {
			os.Setenv("DGVCL_PASSWORD", origPassword)
		}
		embeddedEnv = origEmbeddedEnv
	}()
	
	// Test missing required fields
	os.Unsetenv("DGVCL_USERNAME")
	os.Unsetenv("DGVCL_PASSWORD")
	
	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for missing DGVCL_USERNAME")
	}
	
	// Test with valid config
	os.Setenv("DGVCL_USERNAME", "testuser")
	os.Setenv("DGVCL_PASSWORD", "testpass")
	
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
	
	if cfg.Username != "testuser" {
		t.Errorf("expected username 'testuser' but got %q", cfg.Username)
	}
	
	if cfg.Password != "testpass" {
		t.Errorf("expected password 'testpass' but got %q", cfg.Password)
	}
	
	// Test defaults
	if cfg.MaxLoginRetries != 3 {
		t.Errorf("expected default MaxLoginRetries=3 but got %d", cfg.MaxLoginRetries)
	}
	
	if cfg.FetchInterval != 15*time.Minute {
		t.Errorf("expected default FetchInterval=15m but got %v", cfg.FetchInterval)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "env var set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
		{
			name:         "env var not set",
			key:          "NONEXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}
			
			result := getEnvOrDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("expected %q but got %q", tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		expected     int
	}{
		{
			name:         "valid int",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "25",
			expected:     25,
		},
		{
			name:         "invalid int uses default",
			key:          "TEST_INT_INVALID",
			defaultValue: 10,
			envValue:     "notanumber",
			expected:     10,
		},
		{
			name:         "empty uses default",
			key:          "TEST_INT_EMPTY",
			defaultValue: 10,
			envValue:     "",
			expected:     10,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}
			
			result := getEnvInt(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("expected %d but got %d", tt.expected, result)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Username:     "user",
				Password:     "pass",
				LoginURL:     "http://example.com",
				ComplaintURL: "http://example.com/complaints",
			},
			expectErr: false,
		},
		{
			name: "missing username",
			config: &Config{
				Password:     "pass",
				LoginURL:     "http://example.com",
				ComplaintURL: "http://example.com/complaints",
			},
			expectErr: true,
		},
		{
			name: "missing password",
			config: &Config{
				Username:     "user",
				LoginURL:     "http://example.com",
				ComplaintURL: "http://example.com/complaints",
			},
			expectErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
