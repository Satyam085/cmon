package sfms

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cmon/internal/storage"
)

func withTempCWD(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

func TestKhanpurFeederExcludedFromDefaultConfig(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	for _, f := range cfg.FilterFeeders {
		if strings.Contains(strings.ToLower(f), "khanpur") {
			t.Errorf("expected Khanpur to be removed from FilterFeeders, but found %q", f)
		}
	}

	if _, exists := cfg.FeederSchedules["KHANPUR"]; exists {
		t.Errorf("expected KHANPUR to be removed from FeederSchedules")
	}
}

func TestInterruptionPersistenceAcrossRestart(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer stor.Close()

	cfg := &Config{
		FilterFeeders: []string{"11kV Hill"},
	}

	// 1. First run: feeder is offline
	mon1 := NewMonitor(cfg, nil, nil, nil, stor, nil)
	mon1.cachedSubstations = []Substation{
		{
			SSID: 1,
			Name: "66kV Bhimpore",
			FeederInfo: []FeederInfo{
				{
					FID:         101,
					Name:        "11kV Hill",
					Cat:         7,
					ISACT:       false,
					BMUISACT:    false,
					BMUSerialNo: "BMU101",
					FdrCode:     "322702",
				},
			},
		},
	}

	intTime := time.Date(2026, 8, 18, 14, 0, 0, 0, IST)
	// Manually inject a known interruption time before save
	_ = mon1.EvaluateFeederStates(context.Background(), false)
	mon1.states[101].InterruptedSince = &intTime
	// Save to DB
	_ = stor.SaveSFMSFeeders([]storage.SFMSFeederRecord{
		{
			FID:              101,
			Name:             "11kV Hill",
			Category:         7,
			CategoryName:     "HTEX",
			Is24x7:           true,
			IsOnline:         false,
			InterruptedSince: &intTime,
		},
	})

	// 2. Server restarts: New monitor instance created with existing storage
	mon2 := NewMonitor(cfg, nil, nil, nil, stor, nil)
	if len(mon2.states) != 1 {
		t.Fatalf("expected 1 restored feeder state in mon2, got %d", len(mon2.states))
	}
	st2 := mon2.states[101]
	if st2 == nil || st2.InterruptedSince == nil {
		t.Fatalf("expected feeder 101 to have InterruptedSince restored on startup")
	}
	if st2.InterruptedSince.In(IST).Format("2006-01-02 15:04:05") != "2026-08-18 14:00:00" {
		t.Errorf("got InterruptedSince %s, want 2026-08-18 14:00:00", st2.InterruptedSince.In(IST).Format("2006-01-02 15:04:05"))
	}

	mon2.cachedSubstations = mon1.cachedSubstations

	// Evaluate again while still offline
	_ = mon2.EvaluateFeederStates(context.Background(), false)
	if mon2.states[101].InterruptedSince.In(IST).Format("2006-01-02 15:04:05") != "2026-08-18 14:00:00" {
		t.Errorf("InterruptedSince was overwritten on evaluation after restart: %v", mon2.states[101].InterruptedSince)
	}

	// 3. Dashboard payload calculation check
	payload := mon2.GetDashboardPayload()
	if len(payload.Groups) != 1 || len(payload.Groups[0].Feeders) != 1 {
		t.Fatalf("expected 1 feeder in dashboard payload")
	}
	fItem := payload.Groups[0].Feeders[0]
	if fItem.IsOnline {
		t.Errorf("expected feeder to be offline")
	}
	if fItem.InterruptedSince != "18-08-26 14:00:00" {
		t.Errorf("got InterruptedSince %q in payload, want %q", fItem.InterruptedSince, "18-08-26 14:00:00")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-5 * time.Minute, "0s"},
		{45 * time.Second, "45s"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{3*time.Hour + 20*time.Minute + 5*time.Second, "3h 20m 5s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
