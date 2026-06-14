package storage

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
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

func TestSetMessageIDKeepsMemoryConsistentOnDBFailure(t *testing.T) {
	withTempCWD(t)

	stor, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]Record{{
		ComplaintID: "CMP-1",
		APIID:       "API-1",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	if err := stor.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if err := stor.SetMessageID("CMP-1", "12345"); err == nil {
		t.Fatal("expected SetMessageID to fail after DB close")
	}

	if got := stor.messageIDs["CMP-1"]; got != "" {
		t.Fatalf("message ID should not be cached after failed DB write, got %q", got)
	}
}

func TestSetWAMessageIDKeepsMemoryConsistentOnDBFailure(t *testing.T) {
	withTempCWD(t)

	stor, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]Record{{
		ComplaintID: "CMP-1",
		APIID:       "API-1",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	if err := stor.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if err := stor.SetWAMessageID("CMP-1", "wa-123"); err == nil {
		t.Fatal("expected SetWAMessageID to fail after DB close")
	}

	if got := stor.waMessageIDs["CMP-1"]; got != "" {
		t.Fatalf("WA message ID should not be cached after failed DB write, got %q", got)
	}
	if complaintID, ok := stor.waMessageToComplaint["wa-123"]; ok {
		t.Fatalf("reverse WA index should not be updated after failed DB write, got %q", complaintID)
	}
}

func TestSaveMultiplePersistsDetailFields(t *testing.T) {
	withTempCWD(t)

	stor, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]Record{{
		ComplaintID:  "CMP-DETAIL",
		APIID:        "API-1",
		ConsumerName: "Alice",
		Village:      "Tokarva",
		Belt:         "Bajipura",
		ConsumerNo:   "CONS-1111",
		MobileNo:     "9999999999",
		Address:      "Plot 7, Lane 2",
		Area:         "Bajipura",
		Description:  "no power since morning",
		ComplainDate: "2026-05-09 08:00",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"ConsumerNo", stor.GetConsumerNo("CMP-DETAIL"), "CONS-1111"},
		{"MobileNo", stor.GetMobileNo("CMP-DETAIL"), "9999999999"},
		{"Address", stor.GetAddress("CMP-DETAIL"), "Plot 7, Lane 2"},
		{"Area", stor.GetArea("CMP-DETAIL"), "Bajipura"},
		{"Description", stor.GetDescription("CMP-DETAIL"), "no power since morning"},
		{"ComplainDate", stor.GetComplainDate("CMP-DETAIL"), "2026-05-09 08:00"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}

	// SetDetails should overwrite existing fields without disturbing other data.
	if err := stor.SetDetails("CMP-DETAIL", "CONS-2222", "8888888888", "New Addr", "New Area", "updated desc", "2026-05-10"); err != nil {
		t.Fatalf("SetDetails: %v", err)
	}
	if got := stor.GetConsumerNo("CMP-DETAIL"); got != "CONS-2222" {
		t.Errorf("SetDetails ConsumerNo: got %q, want %q", got, "CONS-2222")
	}
	if got := stor.GetMobileNo("CMP-DETAIL"); got != "8888888888" {
		t.Errorf("SetDetails MobileNo: got %q, want %q", got, "8888888888")
	}
	if got := stor.GetConsumerName("CMP-DETAIL"); got != "Alice" {
		t.Errorf("SetDetails should not touch ConsumerName, got %q", got)
	}
	if got := stor.GetBelt("CMP-DETAIL"); got != "Bajipura" {
		t.Errorf("SetDetails should not touch Belt, got %q", got)
	}
}

// TestReopenIsIdempotent simulates a production upgrade: an existing DB file
// is reopened with the same code path. ensureComplaintColumn must tolerate the
// "column already exists" case so a second startup doesn't fatal-out, and
// stored data must survive the reopen unchanged.
func TestReopenIsIdempotent(t *testing.T) {
	withTempCWD(t)

	// First open — creates the table and runs ensureComplaintColumn for every
	// column. All ALTERs succeed because the columns don't exist yet.
	s1, err := New()
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if err := s1.SaveMultiple([]Record{{
		ComplaintID:  "CMP-1",
		APIID:        "API-1",
		ConsumerName: "Alice",
		Description:  "initial",
	}}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Second open of the same file — every ensureComplaintColumn ALTER now
	// targets a pre-existing column. The function must swallow the duplicate-
	// column error rather than fatal. This is the production-upgrade path.
	s2, err := New()
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	if !s2.Exists("CMP-1") {
		t.Fatalf("data lost after reopen")
	}
	if got := s2.GetDescription("CMP-1"); got != "initial" {
		t.Errorf("Description after reopen: got %q, want %q", got, "initial")
	}
}

// TestUpgradeFromLegacySchema simulates upgrading a real production DB that
// only has the columns from the previous release. The startup path must add
// the missing columns and load existing rows without losing data.
func TestUpgradeFromLegacySchema(t *testing.T) {
	withTempCWD(t)

	// Manually build a "legacy" DB containing the schema as it existed before
	// the detail-field columns were added. This is exactly what's on disk in
	// any environment that ran the previous binary.
	legacyDB, err := sql.Open("sqlite", dbFile+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE complaints (
			complaint_id TEXT PRIMARY KEY,
			tg_message_id TEXT,
			wa_message_id TEXT,
			api_id TEXT,
			consumer_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			village TEXT,
			belt TEXT
		);
		CREATE TABLE pending_resolutions (
			user_id INTEGER PRIMARY KEY,
			complaint_id TEXT,
			message_id TEXT,
			original_text TEXT,
			prompt_message_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO complaints (complaint_id, tg_message_id, api_id, consumer_name, village, belt)
		VALUES ('CMP-LEGACY', '42', 'API-LEGACY', 'Bob', 'Tokarva', 'Bajipura');
	`); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	// Open with the current code — must add the 5 new columns, leave NULL
	// values readable as empty strings, and preserve the legacy row's data.
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if !s.Exists("CMP-LEGACY") {
		t.Fatalf("legacy row lost after upgrade")
	}
	if got := s.GetConsumerName("CMP-LEGACY"); got != "Bob" {
		t.Errorf("legacy ConsumerName after upgrade: got %q, want %q", got, "Bob")
	}
	if got := s.GetVillage("CMP-LEGACY"); got != "Tokarva" {
		t.Errorf("legacy Village after upgrade: got %q, want %q", got, "Tokarva")
	}
	// New columns must read as empty (NULL → "") so dashboard treats them as
	// "needs backfill" rather than crashing.
	if got := s.GetDescription("CMP-LEGACY"); got != "" {
		t.Errorf("new Description on legacy row: got %q, want empty", got)
	}
	if got := s.GetMobileNo("CMP-LEGACY"); got != "" {
		t.Errorf("new MobileNo on legacy row: got %q, want empty", got)
	}

	// And the new SetDetails path must work on the legacy row (this is what
	// the lazy-backfill code path in summary/fetcher.go calls).
	if err := s.SetDetails("CMP-LEGACY", "CONS-L", "777", "addr", "area", "desc", "2026-05-09"); err != nil {
		t.Fatalf("SetDetails on legacy row: %v", err)
	}
	if got := s.GetDescription("CMP-LEGACY"); got != "desc" {
		t.Errorf("after backfill Description: got %q, want %q", got, "desc")
	}
	if got := s.GetConsumerNo("CMP-LEGACY"); got != "CONS-L" {
		t.Errorf("after backfill ConsumerNo: got %q, want %q", got, "CONS-L")
	}
}

func TestRemoveDeletesPendingResolutions(t *testing.T) {
	withTempCWD(t)

	stor, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]Record{{
		ComplaintID: "CMP-1",
		APIID:       "API-1",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	if err := stor.AddPendingResolution(7, PendingResolution{
		ComplaintNumber: "CMP-1",
		MessageID:       "12345",
		OriginalText:    "original",
		PromptMessageID: 99,
	}); err != nil {
		t.Fatalf("add pending resolution: %v", err)
	}

	if err := stor.Remove("CMP-1"); err != nil {
		t.Fatalf("remove complaint: %v", err)
	}

	if _, exists := stor.GetPendingResolution(7); exists {
		t.Fatal("pending resolution should be deleted when complaint is removed")
	}
}

