package storage

import (
	"database/sql"
	"testing"
	"time"
)

func TestSFMSStorage(t *testing.T) {
	withTempCWD(t)

	stor, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer stor.Close()

	// Test SetSFMSToken and GetSFMSToken
	token := "Bearer test-jwt-token-123"
	if err := stor.SetSFMSToken(token); err != nil {
		t.Fatalf("SetSFMSToken failed: %v", err)
	}
	gotToken, err := stor.GetSFMSToken()
	if err != nil {
		t.Fatalf("GetSFMSToken failed: %v", err)
	}
	if gotToken != token {
		t.Errorf("got token %q, want %q", gotToken, token)
	}

	updatedAt, err := stor.GetSFMSTokenUpdatedAt()
	if err != nil {
		t.Fatalf("GetSFMSTokenUpdatedAt failed: %v", err)
	}
	if updatedAt.IsZero() {
		t.Errorf("expected non-zero token updated_at timestamp")
	}

	// Test SaveSFMSFeeders and GetSFMSFeeders
	now := time.Now().Truncate(time.Second)
	feeders := []SFMSFeederRecord{
		{
			FID:            101,
			Name:           "11kV Bahej",
			Category:       2,
			CategoryName:   "AG",
			Is24x7:         false,
			ScheduleStart:  "07:00",
			ScheduleEnd:    "17:00",
			SubstationID:   1,
			SubstationName: "Bhimpore",
			FdrCode:        "322707",
			Device:         "D-101",
			Seq:            1,
			IsActive:       true,
			BMUIsActive:    true,
			CBON:           1,
			CBOFF:          0,
			HasTelemetry:   true,
			BreakerStatus:  "CLOSED",
			IsOnline:       true,
			UpdatedAt:      now,
		},
		{
			FID:            102,
			Name:           "11kV Hill",
			Category:       7,
			CategoryName:   "HTEX",
			Is24x7:         true,
			ScheduleStart:  "00:00",
			ScheduleEnd:    "24:00",
			SubstationID:   1,
			SubstationName: "Bhimpore",
			FdrCode:        "322702",
			Device:         "D-102",
			Seq:            2,
			IsActive:       true,
			BMUIsActive:    true,
			CBON:           0,
			CBOFF:          1,
			HasTelemetry:   true,
			BreakerStatus:  "OPEN",
			IsOnline:       false,
			UpdatedAt:      now,
		},
	}

	if err := stor.SaveSFMSFeeders(feeders); err != nil {
		t.Fatalf("SaveSFMSFeeders failed: %v", err)
	}

	gotFeeders, err := stor.GetSFMSFeeders()
	if err != nil {
		t.Fatalf("GetSFMSFeeders failed: %v", err)
	}
	if len(gotFeeders) != 2 {
		t.Fatalf("expected 2 feeders, got %d", len(gotFeeders))
	}

	// Test InterruptedSince persistence
	intTime := time.Date(2026, 8, 18, 14, 30, 0, 0, IST)
	feeders[1].InterruptedSince = &intTime
	if err := stor.SaveSFMSFeeders(feeders); err != nil {
		t.Fatalf("SaveSFMSFeeders with InterruptedSince failed: %v", err)
	}
	gotFeeders2, err := stor.GetSFMSFeeders()
	if err != nil {
		t.Fatalf("GetSFMSFeeders failed: %v", err)
	}
	var hill *SFMSFeederRecord
	for i := range gotFeeders2 {
		if gotFeeders2[i].FID == 102 {
			hill = &gotFeeders2[i]
		}
	}
	if hill == nil || hill.InterruptedSince == nil {
		t.Fatalf("expected Hill feeder to have InterruptedSince set")
	}
	if hill.InterruptedSince.In(IST).Format("2006-01-02 15:04:05") != "2026-08-18 14:30:00" {
		t.Errorf("got InterruptedSince %s, want 2026-08-18 14:30:00", hill.InterruptedSince.In(IST).Format("2006-01-02 15:04:05"))
	}

	// Test LogSFMSEvent and GetSFMSEvents
	eventTime := time.Date(2026, 8, 18, 14, 30, 0, 0, IST)
	event := SFMSEvent{
		EventType:      "interruption",
		FID:            102,
		FeederName:     "Hill",
		SubstationName: "Bhimpore",
		Category:       "HTEX",
		FdrCode:        "322702",
		Message:        "Feeder Hill interrupted at Bhimpore",
		Timestamp:      eventTime,
	}

	if err := stor.LogSFMSEvent(event); err != nil {
		t.Fatalf("LogSFMSEvent failed: %v", err)
	}

	events, err := stor.GetSFMSEvents(10)
	if err != nil {
		t.Fatalf("GetSFMSEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].FeederName != "Hill" || events[0].EventType != "interruption" {
		t.Errorf("unexpected event: %+v", events[0])
	}
	if events[0].TimestampIST != "18-08-26 14:30:00" {
		t.Errorf("got TimestampIST %q, want %q", events[0].TimestampIST, "18-08-26 14:30:00")
	}
}

func TestAutoMigrationFromLegacyDB(t *testing.T) {
	withTempCWD(t)

	// Step 1: Create a legacy database that ONLY has the old complaints table
	rawDB, err := sql.Open("sqlite", "cmon.db")
	if err != nil {
		t.Fatalf("failed to create raw test DB: %v", err)
	}
	_, err = rawDB.Exec(`
		CREATE TABLE complaints (
			complaint_id TEXT PRIMARY KEY,
			tg_message_id TEXT,
			wa_message_id TEXT,
			api_id TEXT,
			consumer_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE pending_resolutions (
			user_id INTEGER PRIMARY KEY,
			complaint_id TEXT,
			message_id TEXT,
			original_text TEXT,
			prompt_message_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO complaints (complaint_id, api_id, consumer_name) VALUES ('CMP-OLD-1', 'API-OLD-1', 'Ramesh Patel');
	`)
	if err != nil {
		t.Fatalf("failed to seed legacy DB: %v", err)
	}
	rawDB.Close()

	// Step 2: Open with new CMON storage.New() — should run auto-migrations seamlessly
	stor, err := New()
	if err != nil {
		t.Fatalf("New() failed on legacy DB migration: %v", err)
	}
	defer stor.Close()

	// Step 3: Verify existing legacy complaint data is 100% intact
	if !stor.Exists("CMP-OLD-1") {
		t.Errorf("expected legacy complaint CMP-OLD-1 to exist after migration")
	}

	// Step 4: Verify new SFMS tables work without issues
	if err := stor.SetSFMSToken("Bearer migrated-token"); err != nil {
		t.Errorf("SetSFMSToken on migrated DB failed: %v", err)
	}
	tok, err := stor.GetSFMSToken()
	if err != nil || tok != "Bearer migrated-token" {
		t.Errorf("GetSFMSToken got %q, err=%v", tok, err)
	}
}

