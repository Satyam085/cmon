package main

import (
	"os"
	"sync"
	"testing"
	"time"

	"cmon/internal/storage"
)

func withTempCWD(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func TestMarkResolvedComplaintsRemovesWithoutTelegramState(t *testing.T) {
	withTempCWD(t)

	stor, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]storage.Record{{
		ComplaintID:  "CMP-1",
		APIID:        "API-1",
		ConsumerName: "Test Consumer",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	markResolvedComplaints(stor, nil, nil, nil)

	if stor.Exists("CMP-1") {
		t.Fatal("complaint should be removed when it is no longer active, even without Telegram state")
	}
}

func TestWaitWithTimeoutReturnsTrueWhenWaitGroupCompletesInTime(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		wg.Done()
	}()
	if !waitWithTimeout(&wg, 500*time.Millisecond) {
		t.Error("waitWithTimeout returned false despite WaitGroup finishing well before deadline")
	}
}

func TestWaitWithTimeoutReturnsFalseWhenWaitGroupBlocks(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Done() // never finishes during the test window

	if waitWithTimeout(&wg, 50*time.Millisecond) {
		t.Error("waitWithTimeout returned true even though the WaitGroup never finished")
	}
}

func TestParseHHMMToday(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 30, 45, 0, time.Local)

	cases := []struct {
		in     string
		want   string // formatted "15:04" of result
		wantOK bool
	}{
		{"09:00", "09:00", true},
		{"23:59", "23:59", true},
		{"00:00", "00:00", true},
		{"9:00", "", false},   // strict 2-digit hour required
		{"24:00", "", false},  // out of range
		{"12:60", "", false},  // out of range
		{"abcde", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseHHMMToday(tc.in, now)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Format("15:04") != tc.want {
				t.Errorf("got %s, want %s", got.Format("15:04"), tc.want)
			}
			// Date components should be those of `now`.
			if got.Year() != now.Year() || got.Month() != now.Month() || got.Day() != now.Day() {
				t.Errorf("date should be today (%v); got %v", now.Format("2006-01-02"), got.Format("2006-01-02"))
			}
		})
	}
}

func TestNextScheduledFireRollsOverPastTimes(t *testing.T) {
	now := time.Date(2026, 5, 10, 14, 0, 0, 0, time.Local)

	t.Run("only future entry today", func(t *testing.T) {
		// 18:00 is later today; 09:00 already passed → should pick 18:00 today.
		got, ok := nextScheduledFire([]string{"09:00", "18:00"}, now)
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := time.Date(2026, 5, 10, 18, 0, 0, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("all entries already passed today rolls to tomorrow", func(t *testing.T) {
		// Both 09:00 and 12:00 < now=14:00. 09:00 tomorrow is the earlier of
		// the two next firings, so that's what we should get.
		got, ok := nextScheduledFire([]string{"09:00", "12:00"}, now)
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := time.Date(2026, 5, 11, 9, 0, 0, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("equal-to-now is treated as past (must be strictly future)", func(t *testing.T) {
		// 14:00 is exactly now → must roll to tomorrow rather than firing
		// instantly twice in the same minute.
		got, ok := nextScheduledFire([]string{"14:00"}, now)
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := time.Date(2026, 5, 11, 14, 0, 0, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("no valid entries returns ok=false", func(t *testing.T) {
		_, ok := nextScheduledFire([]string{"99:99", "garbage"}, now)
		if ok {
			t.Error("expected ok=false for entries that all fail to parse")
		}
	})

	t.Run("empty list returns ok=false", func(t *testing.T) {
		_, ok := nextScheduledFire(nil, now)
		if ok {
			t.Error("nil schedules should yield ok=false")
		}
	})
}
