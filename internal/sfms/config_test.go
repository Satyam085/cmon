package sfms

import (
	"testing"
	"time"
)

func TestCleanFeederName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"11kV Bahej", "Bahej"},
		{"11 KV Bhimpore", "Bhimpore"},
		{"11kvHill", "Hill"},
		{"Khakhar", "Khakhar"},
		{"  11kV Kumbhiya  ", "Kumbhiya"},
	}

	for _, tt := range tests {
		got := CleanFeederName(tt.input)
		if got != tt.want {
			t.Errorf("CleanFeederName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCleanSubstationName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"66kV Bhimpore", "Bhimpore"},
		{"66 KV Valod", "Valod"},
		{"66kvDegama", "Degama"},
		{"Kelkui", "Kelkui"},
	}

	for _, tt := range tests {
		got := CleanSubstationName(tt.input)
		if got != tt.want {
			t.Errorf("CleanSubstationName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeBearerToken(t *testing.T) {
	if got := NormalizeBearerToken("xyz123"); got != "Bearer xyz123" {
		t.Errorf("got %q, want Bearer xyz123", got)
	}
	if got := NormalizeBearerToken("Bearer xyz123"); got != "Bearer xyz123" {
		t.Errorf("got %q, want Bearer xyz123", got)
	}
	if got := NormalizeBearerToken(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestIsFeederInActiveWindow(t *testing.T) {
	cfg := &Config{
		AGWindowStart: "08:00",
		AGWindowEnd:   "16:00",
		FeederSchedules: map[string]FeederSchedule{
			"BAHEJ": {
				Type:  "AG",
				Start: "07:00",
				End:   "17:00",
			},
			"BHIMPORE": {
				Type:  "JGY",
				Start: "00:00",
				End:   "24:00",
			},
		},
	}

	mon := &Monitor{config: cfg}

	// 10:00 AM IST
	t1 := time.Date(2026, 8, 17, 10, 0, 0, 0, IST)
	isActive, is24x7, fType, _, _ := mon.IsFeederInActiveWindow("11kV Bahej", 2, t1)
	if !isActive || is24x7 || fType != "AG" {
		t.Errorf("Bahej at 10:00: active=%t, 24x7=%t, type=%s", isActive, is24x7, fType)
	}

	// 20:00 (8 PM) IST
	t2 := time.Date(2026, 8, 17, 20, 0, 0, 0, IST)
	isActive, _, _, _, _ = mon.IsFeederInActiveWindow("11kV Bahej", 2, t2)
	if isActive {
		t.Errorf("Bahej at 20:00 should be dormant, got active=%t", isActive)
	}

	// 24x7 JGY feeder
	isActive, is24x7, fType, _, _ = mon.IsFeederInActiveWindow("11kV Bhimpore", 1, t2)
	if !isActive || !is24x7 || fType != "JGY" {
		t.Errorf("Bhimpore at 20:00: active=%t, 24x7=%t, type=%s", isActive, is24x7, fType)
	}
}
