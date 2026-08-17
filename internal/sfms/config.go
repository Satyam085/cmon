package sfms

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed default_config.json
var embeddedDefaultConfig []byte

// IST is the standard Indian Standard Time zone (UTC+5:30).
var IST = time.FixedZone("IST", 5*3600+30*60)

// NowIST returns the current time in Indian Standard Time (UTC+5:30).
func NowIST() time.Time {
	return time.Now().In(IST)
}

// FormatNowIST formats current IST time as "DD-MM-YY HH:MM:SS" (e.g. "17-08-26 11:15:30").
func FormatNowIST() string {
	return time.Now().In(IST).Format("02-01-06 15:04:05")
}

// FormatTimeIST formats a given time in IST as "DD-MM-YY HH:MM:SS".
func FormatTimeIST(t time.Time) string {
	return t.In(IST).Format("02-01-06 15:04:05")
}

// NormalizeBearerToken ensures a token string is prefixed with "Bearer ".
func NormalizeBearerToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(t), "bearer ") {
		return "Bearer " + t
	}
	return t
}

// CleanFeederName strips common voltage prefixes (e.g. "11kV ", "11 KV ") and extra spaces.
func CleanFeederName(name string) string {
	clean := strings.TrimSpace(name)
	lower := strings.ToLower(clean)
	for _, p := range []string{"11kv ", "11 kv ", "11kv", "11 kv"} {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(clean[len(p):])
		}
	}
	return clean
}

// CleanSubstationName strips common voltage prefixes (e.g. "66kV ", "66 KV ") and extra spaces.
func CleanSubstationName(name string) string {
	clean := strings.TrimSpace(name)
	lower := strings.ToLower(clean)
	for _, p := range []string{"66kv ", "66 kv ", "66kv", "66 kv"} {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(clean[len(p):])
		}
	}
	return clean
}

// FeederCategoryName maps category IDs to human-readable names.
func FeederCategoryName(cat int) string {
	switch cat {
	case 1:
		return "JGY"
	case 2:
		return "AG"
	case 4:
		return "Urban"
	case 7:
		return "HTEX"
	case 9:
		return "HT"
	default:
		return fmt.Sprintf("Cat %d", cat)
	}
}

// FeederSchedule defines the monitoring schedule and category for an individual feeder.
type FeederSchedule struct {
	Type       string `json:"type"`                 // e.g. "JGY", "AG", "HTEX", "AGSKY"
	Start      string `json:"start"`                // e.g. "06:00", "07:00", "00:00"
	End        string `json:"end"`                  // e.g. "16:00", "17:00", "24:00"
	Substation string `json:"substation,omitempty"` // e.g. "Bhimpore"
	Code       string `json:"code,omitempty"`       // e.g. "322707"
}

// Config holds configuration parameters for the SFMS monitor.
type Config struct {
	APIURL              string                    `json:"api_url"`
	BearerToken         string                    `json:"bearer_token"`
	Username            string                    `json:"username"`
	Password            string                    `json:"password"`
	LoginURL            string                    `json:"login_url"`
	DepID               string                    `json:"dep_id"`
	DivID               string                    `json:"div_id"`
	PollIntervalSeconds int                       `json:"poll_interval_seconds"`
	EnableColors        bool                      `json:"enable_colors"`
	FilterSubstations   []string                  `json:"filter_substations"`
	FilterFeeders       []string                  `json:"filter_feeders"`
	FeederSchedules     map[string]FeederSchedule `json:"feeder_schedules,omitempty"`
	TelegramBotToken    string                    `json:"telegram_bot_token"`
	TelegramChatID      string                    `json:"telegram_chat_id"`
	AGWindowStart       string                    `json:"ag_window_start"`
	AGWindowEnd         string                    `json:"ag_window_end"`
	ConfigPath          string                    `json:"-"`
}

// LoadConfig reads and parses the JSON configuration file or environment variables.
func LoadConfig(paths ...string) (*Config, error) {
	cfg := Config{
		AGWindowStart:       "08:00",
		AGWindowEnd:         "16:00",
		PollIntervalSeconds: 15,
		EnableColors:        true,
		APIURL:              "https://api.getco-sfms.in/api/SFMS/SFMS_Master_SubStation/Substation_feederInfo?GetCompressed_json=false&IsSequrityVerify=true&PROJCD=sfms",
		LoginURL:            "https://api.getco-sfms.in/api/Token/LCPortalLogin?IsSequrityVerify=true&PROJCD=sfms",
		DepID:               "500059",
		DivID:               "64",
		FeederSchedules:     make(map[string]FeederSchedule),
	}

	if len(embeddedDefaultConfig) > 0 {
		_ = json.Unmarshal(embeddedDefaultConfig, &cfg)
	}

	searchPaths := []string{"smfs/config.json", "config.json"}
	if len(paths) > 0 && paths[0] != "" {
		searchPaths = append([]string{paths[0]}, searchPaths...)
	}

	for _, p := range searchPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			if err := json.Unmarshal(data, &cfg); err == nil {
				cfg.ConfigPath = p
				break
			}
		}
	}

	overrideEnv := func(target *string, key string) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			*target = v
		}
	}
	overrideEnv(&cfg.Username, "SFMS_USERNAME")
	overrideEnv(&cfg.Password, "SFMS_PASSWORD")
	overrideEnv(&cfg.DepID, "SFMS_DEP_ID")
	overrideEnv(&cfg.DivID, "SFMS_DIV_ID")
	overrideEnv(&cfg.AGWindowStart, "SFMS_AG_WINDOW_START")
	overrideEnv(&cfg.AGWindowEnd, "SFMS_AG_WINDOW_END")

	if pi := os.Getenv("SFMS_POLL_INTERVAL"); pi != "" {
		if s, err := strconv.Atoi(strings.TrimSpace(pi)); err == nil && s > 0 {
			cfg.PollIntervalSeconds = s
		}
	}

	if envToken := os.Getenv("SFMS_BEARER_TOKEN"); envToken != "" {
		cfg.BearerToken = NormalizeBearerToken(envToken)
	} else if cfg.BearerToken != "" {
		cfg.BearerToken = NormalizeBearerToken(cfg.BearerToken)
	} else if tokenData, err := os.ReadFile("token.txt"); err == nil {
		cfg.BearerToken = NormalizeBearerToken(string(tokenData))
	} else if tokenData, err := os.ReadFile("smfs/token.txt"); err == nil {
		cfg.BearerToken = NormalizeBearerToken(string(tokenData))
	}

	return &cfg, nil
}
