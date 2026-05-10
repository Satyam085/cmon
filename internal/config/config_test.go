package config

import (
	"strings"
	"testing"
	"time"
)

// The helper functions are the heart of env precedence. The embedded .env in
// the binary plus os.Setenv (used by LoadConfig itself) makes hermetic tests
// of LoadConfig() awkward; the helpers, by contrast, only read os.Getenv and
// are trivially isolatable with t.Setenv.

func TestGetEnvOrDefault(t *testing.T) {
	const key = "CMON_TEST_OR_DEFAULT"

	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv(key, "from-env")
		if got := getEnvOrDefault(key, "fallback"); got != "from-env" {
			t.Errorf("got %q, want from-env", got)
		}
	})

	t.Run("returns default when env unset", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getEnvOrDefault(key, "fallback"); got != "fallback" {
			t.Errorf("got %q, want fallback", got)
		}
	})

	t.Run("treats empty env value as unset", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getEnvOrDefault(key, "fallback"); got != "fallback" {
			t.Errorf("empty env should fall through to default; got %q", got)
		}
	})
}

func TestGetEnvInt(t *testing.T) {
	const key = "CMON_TEST_INT"

	cases := []struct {
		name string
		env  string
		def  int
		want int
	}{
		{"valid integer", "42", 7, 42},
		{"zero", "0", 7, 0},
		{"negative integer", "-3", 7, -3},
		{"invalid string falls back", "not a number", 7, 7},
		{"empty falls back", "", 7, 7},
		{"float-looking string falls back", "3.14", 7, 7},
		{"trailing junk falls back", "10x", 7, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(key, tc.env)
			if got := getEnvInt(key, tc.def); got != tc.want {
				t.Errorf("getEnvInt(%q, %d) = %d, want %d", tc.env, tc.def, got, tc.want)
			}
		})
	}
}

func TestGetEnvFloat(t *testing.T) {
	const key = "CMON_TEST_FLOAT"

	cases := []struct {
		name string
		env  string
		def  float64
		want float64
	}{
		{"valid float", "3.14", 1.0, 3.14},
		{"valid integer parses as float", "5", 1.0, 5.0},
		{"negative", "-2.5", 1.0, -2.5},
		{"invalid falls back", "junk", 1.0, 1.0},
		{"empty falls back", "", 1.0, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(key, tc.env)
			if got := getEnvFloat(key, tc.def); got != tc.want {
				t.Errorf("getEnvFloat(%q, %f) = %f, want %f", tc.env, tc.def, got, tc.want)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	const key = "CMON_TEST_DUR"

	cases := []struct {
		name string
		env  string
		def  time.Duration
		want time.Duration
	}{
		{"seconds", "5s", time.Minute, 5 * time.Second},
		{"minutes", "10m", time.Minute, 10 * time.Minute},
		{"compound", "1h30m", time.Minute, 90 * time.Minute},
		{"milliseconds", "250ms", time.Minute, 250 * time.Millisecond},
		{"unitless number falls back", "5", time.Minute, time.Minute},
		{"garbage falls back", "soon", time.Minute, time.Minute},
		{"empty falls back", "", time.Minute, time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(key, tc.env)
			if got := getEnvDuration(key, tc.def); got != tc.want {
				t.Errorf("getEnvDuration(%q) = %s, want %s", tc.env, got, tc.want)
			}
		})
	}
}

// TestValidateRejectsMissingFields covers each branch of Validate().
// Validate() itself takes a *Config so it's hermetic — no env state,
// no embedded .env, no godotenv. Pure data-in / error-out.
func TestValidateRejectsMissingFields(t *testing.T) {
	good := func() *Config {
		return &Config{
			Username:       "u",
			Password:       "p",
			LoginURL:       "https://x/",
			ComplaintURL:   "https://x/dash",
			MaxPages:       5,
			WorkerPoolSize: 10,
		}
	}

	t.Run("happy path passes", func(t *testing.T) {
		if err := good().Validate(); err != nil {
			t.Errorf("baseline good config should pass; got %v", err)
		}
	})

	t.Run("empty username errors", func(t *testing.T) {
		c := good()
		c.Username = ""
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "DGVCL_USERNAME") {
			t.Errorf("missing username should error mentioning DGVCL_USERNAME; got %v", err)
		}
	})

	t.Run("empty password errors", func(t *testing.T) {
		c := good()
		c.Password = ""
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "DGVCL_PASSWORD") {
			t.Errorf("missing password should error mentioning DGVCL_PASSWORD; got %v", err)
		}
	})

	t.Run("empty login url errors", func(t *testing.T) {
		c := good()
		c.LoginURL = ""
		if err := c.Validate(); err == nil {
			t.Error("empty LoginURL should error")
		}
	})

	t.Run("empty complaint url errors", func(t *testing.T) {
		c := good()
		c.ComplaintURL = ""
		if err := c.Validate(); err == nil {
			t.Error("empty ComplaintURL should error")
		}
	})

	t.Run("zero max pages errors", func(t *testing.T) {
		c := good()
		c.MaxPages = 0
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "MAX_PAGES") {
			t.Errorf("MaxPages=0 should error mentioning MAX_PAGES; got %v", err)
		}
	})

	t.Run("negative max pages errors", func(t *testing.T) {
		c := good()
		c.MaxPages = -1
		if err := c.Validate(); err == nil {
			t.Error("negative MaxPages should error")
		}
	})

	t.Run("zero worker pool errors", func(t *testing.T) {
		c := good()
		c.WorkerPoolSize = 0
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "WORKER_POOL_SIZE") {
			t.Errorf("WorkerPoolSize=0 should error mentioning WORKER_POOL_SIZE; got %v", err)
		}
	})
}

// TestLoadConfigEnvOverridesEmbedded covers the env-var precedence rule: a
// value set in the process environment must override whatever the embedded
// .env file ships with. Critical for production deployments that point the
// binary at staging credentials via env vars.
func TestLoadConfigEnvOverridesEmbedded(t *testing.T) {
	t.Setenv("DGVCL_USERNAME", "override-user")
	t.Setenv("DGVCL_PASSWORD", "override-pass")
	t.Setenv("TELEGRAM_BOT_TOKEN", "override-bot-token")
	t.Setenv("HEALTH_CHECK_PORT", "9999")
	t.Setenv("MAX_PAGES", "11")
	t.Setenv("FETCH_INTERVAL", "7m")
	t.Setenv("API_RATE_LIMIT_RPS", "0.5")
	t.Setenv("WHATSAPP_RESOLVE_ENABLED", "true")
	t.Setenv("LOG_FORMAT", "json")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Username != "override-user" {
		t.Errorf("Username: got %q, want override-user", cfg.Username)
	}
	if cfg.Password != "override-pass" {
		t.Errorf("Password: got %q, want override-pass", cfg.Password)
	}
	if cfg.TelegramBotToken != "override-bot-token" {
		t.Errorf("TelegramBotToken: got %q, want override-bot-token", cfg.TelegramBotToken)
	}
	if cfg.HealthCheckPort != "9999" {
		t.Errorf("HealthCheckPort: got %q, want 9999", cfg.HealthCheckPort)
	}
	if cfg.MaxPages != 11 {
		t.Errorf("MaxPages: got %d, want 11", cfg.MaxPages)
	}
	if cfg.FetchInterval != 7*time.Minute {
		t.Errorf("FetchInterval: got %s, want 7m", cfg.FetchInterval)
	}
	if cfg.APIRateLimitRPS != 0.5 {
		t.Errorf("APIRateLimitRPS: got %v, want 0.5", cfg.APIRateLimitRPS)
	}
	if !cfg.WhatsAppResolveEnabled {
		t.Errorf("WhatsAppResolveEnabled: got false, want true")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat: got %q, want json", cfg.LogFormat)
	}
}

// TestLoadConfigEmbeddedFallbackUsedWhenEnvUnset confirms the precedence
// chain's middle tier: when no env var is set, values come from the embedded
// .env. We pick a couple of distinctive values that the embedded file ships.
// This lets a deployment without env vars still boot with sensible config.
func TestLoadConfigEmbeddedFallbackUsedWhenEnvUnset(t *testing.T) {
	// Worker pool is 5 in the embedded .env (would be 10 by hard-coded default).
	// If LoadConfig ever stops loading the embedded file, this drops to 10
	// and the test fails — the canary we want.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.WorkerPoolSize != 5 {
		t.Errorf("WorkerPoolSize from embedded .env: got %d, want 5 (embedded value)", cfg.WorkerPoolSize)
	}
	// HEALTH_CHECK_PORT is in the embedded .env at 8080 (which also matches the
	// hard-coded default, so this row alone wouldn't catch a regression — kept
	// for documentation).
	if cfg.HealthCheckPort != "8080" {
		t.Errorf("HealthCheckPort: got %q, want 8080", cfg.HealthCheckPort)
	}
}

// TestLoadConfigHardcodedDefaultUsedWhenAbsentFromEnvAndEmbedded verifies
// the bottom tier: a key the embedded .env does NOT set falls all the way to
// the hard-coded default in LoadConfig. LOG_FORMAT is the cleanest candidate
// because it was added recently and isn't in the embedded file.
func TestLoadConfigHardcodedDefaultUsedWhenAbsentFromEnvAndEmbedded(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat default: got %q, want text", cfg.LogFormat)
	}
}

// TestLoadConfigInvalidDurationFallsBackToDefault verifies that a bad
// duration string in the environment doesn't crash boot — it falls back to
// the documented default. This protects deployments where someone fat-fingers
// FETCH_INTERVAL=15min instead of 15m.
func TestLoadConfigInvalidDurationFallsBackToDefault(t *testing.T) {
	t.Setenv("DGVCL_USERNAME", "u")
	t.Setenv("DGVCL_PASSWORD", "p")
	t.Setenv("FETCH_INTERVAL", "15min") // invalid — Go expects "15m"

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.FetchInterval != 15*time.Minute {
		t.Errorf("invalid FETCH_INTERVAL should fall back to 15m default; got %s", cfg.FetchInterval)
	}
}

func TestParseBeltRoutes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty", "", nil},
		{"whitespace", "   ", nil},
		{"single pair", "dahod=-1001234", map[string]string{"dahod": "-1001234"}},
		{"two pairs", "dahod=-1001234,bajipura=-1005678", map[string]string{
			"dahod":    "-1001234",
			"bajipura": "-1005678",
		}},
		{"surrounding whitespace", "  dahod = -1001234 , bajipura = -1005678 ", map[string]string{
			"dahod":    "-1001234",
			"bajipura": "-1005678",
		}},
		{"key lowercased", "DAHOD=123,Bajipura=456", map[string]string{
			"dahod":    "123",
			"bajipura": "456",
		}},
		{"missing equals dropped", "dahod 123,bajipura=456", map[string]string{
			"bajipura": "456",
		}},
		{"empty key dropped", "=123,dahod=456", map[string]string{
			"dahod": "456",
		}},
		{"empty value dropped", "dahod=,bajipura=456", map[string]string{
			"bajipura": "456",
		}},
		{"trailing equals dropped", "dahod=", nil},
		{"only invalid tokens returns nil", "garbage,more garbage", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBeltRoutes(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("[%s]: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseScheduleList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace", "  ", nil},
		{"single", "09:00", []string{"09:00"}},
		{"two", "09:00,18:00", []string{"09:00", "18:00"}},
		{"two with spaces", "09:00 , 18:00 ", []string{"09:00", "18:00"}},
		{"drops invalid token", "09:00, garbage, 18:00", []string{"09:00", "18:00"}},
		{"drops out-of-range hour", "24:00, 23:59", []string{"23:59"}},
		{"drops out-of-range minute", "12:60", nil},
		{"drops short hour", "9:00", nil},
		{"drops short minute", "09:0", nil},
		{"drops trailing empty token", "09:00,,", []string{"09:00"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseScheduleList(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d]: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestLoadConfigBoolFlagOnlyTrueLiteral confirms WHATSAPP_RESOLVE_ENABLED is
// strict "true" — anything other than that exact string is false. The
// strictness is intentional: a flag that mutates external state should reject
// near-misses rather than silently enabling itself.
//
// Note: empty values fall through to the embedded .env (which ships
// WHATSAPP_RESOLVE_ENABLED=true), so we don't include "" here. The empty
// case is part of the precedence chain, not the bool-parsing contract.
func TestLoadConfigBoolFlagOnlyTrueLiteral(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"True", false},
		{"TRUE", false},
		{"1", false},
		{"yes", false},
		{"false", false},
	}
	for _, tc := range cases {
		t.Run("WHATSAPP_RESOLVE_ENABLED="+tc.val, func(t *testing.T) {
			t.Setenv("WHATSAPP_RESOLVE_ENABLED", tc.val)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.WhatsAppResolveEnabled != tc.want {
				t.Errorf("WHATSAPP_RESOLVE_ENABLED=%q: got %v, want %v",
					tc.val, cfg.WhatsAppResolveEnabled, tc.want)
			}
		})
	}
}
