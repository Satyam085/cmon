// Package storage provides persistent and in-memory storage for complaint data.
//
// This package implements a two-tier storage system:
//  1. SQLite database for persistent storage (survives restarts)
//  2. In-memory cache for fast lookups (O(1) instead of O(n) DB queries)
//
// Thread-safety:
//   - All operations are protected by a RWMutex
//   - Safe for concurrent access from multiple goroutines
//
// Migration:
//   - On first run, it automatically migrates existing complaints.csv to SQLite
package storage

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	legacyCSVFile = "complaints.csv"
	dbFile        = "cmon.db"
)

// Record represents a single complaint record with all associated data.
type Record struct {
	ComplaintID  string
	MessageID    string
	WAMessageID  string
	APIID        string
	ConsumerName string

	// Cached complaint detail fields (sourced from the DGVCL detail API
	// during scrape, used to render the dashboard without re-fetching).
	MobileNo     string
	Address      string // exact_location
	Area         string
	Description  string
	ComplainDate string
}

// Storage provides thread-safe storage for complaint data.
type Storage struct {
	mu                   sync.RWMutex
	db                   *sql.DB
	seen                 map[string]bool   // complaintID → exists
	messageIDs           map[string]string // complaintID → Telegram message ID
	waMessageIDs         map[string]string // complaintID → WhatsApp message ID
	waMessageToComplaint map[string]string // waMessageID → complaintID (Reverse lookup)
	apiIDs               map[string]string // complaintID → API ID
	consumerNames        map[string]string // complaintID → Consumer name
	mobileNos            map[string]string // complaintID → mobile number
	addresses            map[string]string // complaintID → exact location
	areas                map[string]string // complaintID → area
	descriptions         map[string]string // complaintID → description
	complainDates        map[string]string // complaintID → complain_date
}

// PendingResolution stores info about a complaint awaiting resolution note
type PendingResolution struct {
	ComplaintNumber string
	MessageID       string
	OriginalText    string
	PromptMessageID int
}

// New creates a new Storage instance, connects to SQLite, and loads into memory.
// It also handles the one-time migration from complaints.csv if it exists.
//
// Boot-path failures (open DB, configure pragmas, create tables, load existing
// rows) crash via log.Fatalf because they leave the process in an unrecoverable
// state. Recoverable schema-evolution failures (column-ensure) are returned as
// an error so the caller can decide.
func New() (*Storage, error) {
	s := &Storage{
		seen:                 make(map[string]bool),
		messageIDs:           make(map[string]string),
		waMessageIDs:         make(map[string]string),
		waMessageToComplaint: make(map[string]string),
		apiIDs:               make(map[string]string),
		consumerNames:        make(map[string]string),
		mobileNos:            make(map[string]string),
		addresses:            make(map[string]string),
		areas:                make(map[string]string),
		descriptions:         make(map[string]string),
		complainDates:        make(map[string]string),
	}

	// Connect to SQLite
	db, err := sql.Open("sqlite", dbFile+"?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database %s: %v", dbFile, err)
	}

	importTime := time.Now()
	_ = importTime // for time package use

	// SQLite is a single-file database, so multiple writer connections can easily
	// trip "database is locked" under concurrent message processing. Keep one
	// shared connection and let busy_timeout wait briefly for transient locks.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	s.db = db

	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		log.Fatalf("❌ Failed to configure SQLite pragmas: %v", err)
	}

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS complaints (
			complaint_id TEXT PRIMARY KEY,
			tg_message_id TEXT,
			wa_message_id TEXT,
			api_id TEXT,
			consumer_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS pending_resolutions (
			user_id INTEGER PRIMARY KEY,
			complaint_id TEXT,
			message_id TEXT,
			original_text TEXT,
			prompt_message_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("❌ Failed to create tables: %v", err)
	}

	for _, col := range []struct{ name, typ string }{
		{"mobile_no", "TEXT"},
		{"address", "TEXT"},
		{"area", "TEXT"},
		{"description", "TEXT"},
		{"complain_date", "TEXT"},
	} {
		if err := s.ensureComplaintColumn(col.name, col.typ); err != nil {
			return nil, err
		}
	}

	// Run migration from old complaints.csv if needed
	s.migrateFromCSV()

	// Load data from DB into memory maps
	s.loadFromDB()

	return s, nil
}

// migrateFromCSV parses the legacy complaints.csv file, inserts all records
// into SQLite, and renames the CSV to .bak to prevent re-migration.
func (s *Storage) migrateFromCSV() {
	if _, err := os.Stat(legacyCSVFile); os.IsNotExist(err) {
		return // No CSV file to migrate
	}

	log.Println("🔄 Found legacy complaints.csv. Migrating to SQLite...")

	file, err := os.Open(legacyCSVFile)
	if err != nil {
		log.Printf("⚠️  Failed to open %s for migration: %v", legacyCSVFile, err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("⚠️  Failed to read CSV for migration: %v", err)
		return
	}

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("⚠️  Failed to begin migration transaction: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO complaints (complaint_id, tg_message_id, wa_message_id, api_id, consumer_name) 
		VALUES (?, ?, '', ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		log.Printf("⚠️  Failed to prepare migration statement: %v", err)
		return
	}
	defer stmt.Close()

	migratedCount := 0
	for i, record := range records {
		if i == 0 && len(record) > 0 && (record[0] == "ComplaintID" || record[0] == "complaint_id") {
			continue // Skip header
		}
		if len(record) < 1 {
			continue
		}

		complaintID := record[0]
		tgMessageID := ""
		apiID := ""
		consumerName := ""

		if len(record) >= 2 {
			tgMessageID = record[1]
		}
		if len(record) >= 3 {
			apiID = record[2]
		}
		if len(record) >= 4 {
			consumerName = record[3]
		}

		_, err := stmt.Exec(complaintID, tgMessageID, apiID, consumerName)
		if err != nil {
			log.Printf("⚠️  Failed to migrate record %s: %v", complaintID, err)
			continue
		}
		migratedCount++
	}

	if err := tx.Commit(); err != nil {
		log.Printf("⚠️  Failed to commit migration transaction: %v", err)
		return
	}

	log.Printf("✅ Migrated %d complaints to SQLite.", migratedCount)

	// Rename CSV to prevent re-migration
	backupFile := legacyCSVFile + ".bak"
	file.Close() // Must close before renaming on Windows (safe to call twice due to defer)
	if err := os.Rename(legacyCSVFile, backupFile); err != nil {
		log.Printf("⚠️  Failed to backup CSV to %s: %v. Please delete %s manually.", backupFile, err, legacyCSVFile)
	} else {
		log.Printf("   Old file renamed to %s", backupFile)
	}
}

// loadFromDB loads all complaint data from SQLite into the in-memory maps.
func (s *Storage) loadFromDB() {
	rows, err := s.db.Query(`SELECT complaint_id, tg_message_id, wa_message_id, api_id, consumer_name, mobile_no, address, area, description, complain_date FROM complaints`)
	if err != nil {
		log.Fatalf("❌ Failed to query database on load: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var complaintID, tgMessageID, waMessageID, apiID, consumerName sql.NullString
		var mobileNo, address, area, description, complainDate sql.NullString
		if err := rows.Scan(&complaintID, &tgMessageID, &waMessageID, &apiID, &consumerName, &mobileNo, &address, &area, &description, &complainDate); err != nil {
			log.Printf("⚠️  Failed to scan row on load: %v", err)
			continue
		}

		if complaintID.Valid && complaintID.String != "" {
			s.seen[complaintID.String] = true
			if tgMessageID.Valid {
				s.messageIDs[complaintID.String] = tgMessageID.String
			}
			if waMessageID.Valid && waMessageID.String != "" {
				s.waMessageIDs[complaintID.String] = waMessageID.String
				s.waMessageToComplaint[waMessageID.String] = complaintID.String
			}
			if apiID.Valid {
				s.apiIDs[complaintID.String] = apiID.String
			}
			if consumerName.Valid {
				s.consumerNames[complaintID.String] = consumerName.String
			}
			if mobileNo.Valid {
				s.mobileNos[complaintID.String] = mobileNo.String
			}
			if address.Valid {
				s.addresses[complaintID.String] = address.String
			}
			if area.Valid {
				s.areas[complaintID.String] = area.String
			}
			if description.Valid {
				s.descriptions[complaintID.String] = description.String
			}
			if complainDate.Valid {
				s.complainDates[complaintID.String] = complainDate.String
			}
			count++
		}
	}

	if err := rows.Err(); err != nil {
		log.Printf("⚠️  Row iteration error during load: %v", err)
	}

	log.Printf("📚 Loaded %d previously seen complaints from database", count)
}

// IsNew checks if a complaint ID has been seen before (O(1) memory lookup).
func (s *Storage) IsNew(complaintID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.seen[complaintID]
}

// MarkAsSeen marks a complaint as seen in memory only.
func (s *Storage) MarkAsSeen(complaintID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[complaintID] = true
}

// GetMessageID retrieves the Telegram message ID for a complaint.
func (s *Storage) GetMessageID(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messageIDs[complaintID]
}

// GetWAMessageID retrieves the WhatsApp message ID for a complaint.
func (s *Storage) GetWAMessageID(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.waMessageIDs[complaintID]
}

// SetMessageID updates both memory and DB with a new Telegram message ID.
func (s *Storage) SetMessageID(complaintID, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.seen[complaintID] {
		return fmt.Errorf("complaint %s not found in storage", complaintID)
	}

	if _, err := s.db.Exec(`UPDATE complaints SET tg_message_id = ? WHERE complaint_id = ?`, messageID, complaintID); err != nil {
		log.Printf("⚠️  Failed to persist Telegram message ID for %s: %v", complaintID, err)
		return err
	}

	s.messageIDs[complaintID] = messageID
	return nil
}

// SetWAMessageID updates both memory and DB with a new WhatsApp Message ID.
// This is called asynchronously when a WA message is successfully sent.
func (s *Storage) SetWAMessageID(complaintID, waMessageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Need existence check so we don't save WA message ID if complaint is bad or deleted
	if !s.seen[complaintID] {
		return fmt.Errorf("complaint %s not found in storage", complaintID)
	}

	// Update DB before memory so the in-memory reverse index never gets ahead of
	// the persisted source of truth.
	if _, err := s.db.Exec(`UPDATE complaints SET wa_message_id = ? WHERE complaint_id = ?`, waMessageID, complaintID); err != nil {
		log.Printf("⚠️  Failed to persist WA message ID for %s: %v", complaintID, err)
		return err
	}

	if oldWAMessageID := s.waMessageIDs[complaintID]; oldWAMessageID != "" && oldWAMessageID != waMessageID {
		delete(s.waMessageToComplaint, oldWAMessageID)
	}
	s.waMessageIDs[complaintID] = waMessageID
	if waMessageID != "" {
		s.waMessageToComplaint[waMessageID] = complaintID
	}
	return nil
}

// GetComplaintIDByWAMessageID does a reverse lookup from WhatsApp Message ID to Complaint ID.
// Used by the WhatsApp reply-to-resolve parser.
func (s *Storage) GetComplaintIDByWAMessageID(waMessageID string) (string, bool) {
	// First check memory map for speed
	s.mu.RLock()
	if cid, exists := s.waMessageToComplaint[waMessageID]; exists {
		s.mu.RUnlock()
		return cid, true
	}
	s.mu.RUnlock()

	// Fallback to DB (in case of memory desync)
	var complaintID string
	err := s.db.QueryRow(`SELECT complaint_id FROM complaints WHERE wa_message_id = ?`, waMessageID).Scan(&complaintID)
	if err == sql.ErrNoRows || err != nil {
		return "", false
	}
	// Opportunistic cache fill
	s.mu.Lock()
	s.waMessageIDs[complaintID] = waMessageID
	s.waMessageToComplaint[waMessageID] = complaintID
	s.mu.Unlock()

	return complaintID, true
}

// GetAPIID retrieves the API ID for a complaint.
func (s *Storage) GetAPIID(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiIDs[complaintID]
}

// GetConsumerName retrieves the consumer name for a complaint.
func (s *Storage) GetConsumerName(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consumerNames[complaintID]
}

// GetMobileNo retrieves the cached mobile number for a complaint.
func (s *Storage) GetMobileNo(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mobileNos[complaintID]
}

// GetAddress retrieves the cached exact-location address for a complaint.
func (s *Storage) GetAddress(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addresses[complaintID]
}

// GetArea retrieves the cached area for a complaint.
func (s *Storage) GetArea(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.areas[complaintID]
}

// GetDescription retrieves the cached description for a complaint.
func (s *Storage) GetDescription(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.descriptions[complaintID]
}

// GetComplainDate retrieves the cached complain date for a complaint.
func (s *Storage) GetComplainDate(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.complainDates[complaintID]
}

// SetDetails persists the cached complaint detail fields for a known complaint.
//
// Used by the dashboard layer to lazy-backfill rows that pre-date the schema
// change (or whose details were not captured during their original scrape).
// All fields are written atomically; the in-memory cache is only updated
// after the DB write succeeds so memory never gets ahead of disk.
func (s *Storage) SetDetails(complaintID, mobileNo, address, area, description, complainDate string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.seen[complaintID] {
		return fmt.Errorf("complaint %s not found in storage", complaintID)
	}

	if _, err := s.db.Exec(`
		UPDATE complaints
		SET mobile_no = ?, address = ?, area = ?, description = ?, complain_date = ?
		WHERE complaint_id = ?
	`, mobileNo, address, area, description, complainDate, complaintID); err != nil {
		return err
	}

	s.mobileNos[complaintID] = mobileNo
	s.addresses[complaintID] = address
	s.areas[complaintID] = area
	s.descriptions[complaintID] = description
	s.complainDates[complaintID] = complainDate
	return nil
}

// Exists checks if a complaint exists in memory.
func (s *Storage) Exists(complaintID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seen[complaintID]
}

// GetAllSeenComplaints returns a list of all active complaint IDs.
func (s *Storage) GetAllSeenComplaints() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	complaints := make([]string, 0, len(s.seen))
	for id := range s.seen {
		complaints = append(complaints, id)
	}
	return complaints
}

// SaveMultiple atomically inserts NEW records into SQLite and updates memory.
// Existing records are left untouched in the DB (INSERT OR IGNORE) to preserve
// wa_message_id and other previously saved values.
func (s *Storage) SaveMultiple(records []Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO complaints (complaint_id, tg_message_id, wa_message_id, api_id, consumer_name, mobile_no, address, area, description, complain_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(complaint_id) DO UPDATE SET
			tg_message_id = CASE
				WHEN excluded.tg_message_id != '' THEN excluded.tg_message_id
				ELSE complaints.tg_message_id
			END,
			wa_message_id = CASE
				WHEN excluded.wa_message_id != '' THEN excluded.wa_message_id
				ELSE complaints.wa_message_id
			END,
			api_id = CASE
				WHEN excluded.api_id != '' THEN excluded.api_id
				ELSE complaints.api_id
			END,
			consumer_name = CASE
				WHEN excluded.consumer_name != '' THEN excluded.consumer_name
				ELSE complaints.consumer_name
			END,
			mobile_no = CASE
				WHEN excluded.mobile_no != '' THEN excluded.mobile_no
				ELSE complaints.mobile_no
			END,
			address = CASE
				WHEN excluded.address != '' THEN excluded.address
				ELSE complaints.address
			END,
			area = CASE
				WHEN excluded.area != '' THEN excluded.area
				ELSE complaints.area
			END,
			description = CASE
				WHEN excluded.description != '' THEN excluded.description
				ELSE complaints.description
			END,
			complain_date = CASE
				WHEN excluded.complain_date != '' THEN excluded.complain_date
				ELSE complaints.complain_date
			END
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		if _, err := stmt.Exec(r.ComplaintID, r.MessageID, r.WAMessageID, r.APIID, r.ConsumerName, r.MobileNo, r.Address, r.Area, r.Description, r.ComplainDate); err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Update memory maps (safe to overwrite — same data for new records;
	// for duplicates we still want the latest in-memory state).
	for _, r := range records {
		s.seen[r.ComplaintID] = true
		// Only set tg_message_id in memory if we have one (don't blank existing)
		if r.MessageID != "" {
			s.messageIDs[r.ComplaintID] = r.MessageID
		}
		if r.WAMessageID != "" {
			if oldWAMessageID := s.waMessageIDs[r.ComplaintID]; oldWAMessageID != "" && oldWAMessageID != r.WAMessageID {
				delete(s.waMessageToComplaint, oldWAMessageID)
			}
			s.waMessageIDs[r.ComplaintID] = r.WAMessageID
			s.waMessageToComplaint[r.WAMessageID] = r.ComplaintID
		}
		if r.APIID != "" {
			s.apiIDs[r.ComplaintID] = r.APIID
		}
		if r.ConsumerName != "" {
			s.consumerNames[r.ComplaintID] = r.ConsumerName
		}
		if r.MobileNo != "" {
			s.mobileNos[r.ComplaintID] = r.MobileNo
		}
		if r.Address != "" {
			s.addresses[r.ComplaintID] = r.Address
		}
		if r.Area != "" {
			s.areas[r.ComplaintID] = r.Area
		}
		if r.Description != "" {
			s.descriptions[r.ComplaintID] = r.Description
		}
		if r.ComplainDate != "" {
			s.complainDates[r.ComplaintID] = r.ComplainDate
		}
	}

	return nil
}

// Remove permanently deletes a complaint from SQLite and memory.
func (s *Storage) Remove(complaintID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM pending_resolutions WHERE complaint_id = ?`, complaintID); err != nil {
		tx.Rollback()
		return err
	}

	if _, err := tx.Exec(`DELETE FROM complaints WHERE complaint_id = ?`, complaintID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Remove WA message ID from reverse index
	if waMsgID, ok := s.waMessageIDs[complaintID]; ok && waMsgID != "" {
		delete(s.waMessageToComplaint, waMsgID)
	}

	delete(s.seen, complaintID)
	delete(s.messageIDs, complaintID)
	delete(s.waMessageIDs, complaintID)
	delete(s.apiIDs, complaintID)
	delete(s.consumerNames, complaintID)
	delete(s.mobileNos, complaintID)
	delete(s.addresses, complaintID)
	delete(s.areas, complaintID)
	delete(s.descriptions, complaintID)
	delete(s.complainDates, complaintID)

	return nil
}

// RemoveIfExists conditionally deletes a complaint from SQLite and memory.
// Returns true if deleted, false if it didn't exist.
func (s *Storage) RemoveIfExists(complaintID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.seen[complaintID] {
		return false, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}

	if _, err := tx.Exec(`DELETE FROM pending_resolutions WHERE complaint_id = ?`, complaintID); err != nil {
		tx.Rollback()
		return false, err
	}

	if _, err := tx.Exec(`DELETE FROM complaints WHERE complaint_id = ?`, complaintID); err != nil {
		tx.Rollback()
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	// Remove WA message ID from reverse index
	if waMsgID, ok := s.waMessageIDs[complaintID]; ok && waMsgID != "" {
		delete(s.waMessageToComplaint, waMsgID)
	}

	delete(s.seen, complaintID)
	delete(s.messageIDs, complaintID)
	delete(s.waMessageIDs, complaintID)
	delete(s.apiIDs, complaintID)
	delete(s.consumerNames, complaintID)
	delete(s.mobileNos, complaintID)
	delete(s.addresses, complaintID)
	delete(s.areas, complaintID)
	delete(s.descriptions, complaintID)
	delete(s.complainDates, complaintID)

	return true, nil
}

// GetPendingResolution retrieves a pending resolution from SQLite.
func (s *Storage) GetPendingResolution(userID int64) (PendingResolution, bool) {
	var pr PendingResolution
	err := s.db.QueryRow(`
		SELECT complaint_id, message_id, original_text, prompt_message_id
		FROM pending_resolutions
		WHERE user_id = ?
	`, userID).Scan(&pr.ComplaintNumber, &pr.MessageID, &pr.OriginalText, &pr.PromptMessageID)
	if err == sql.ErrNoRows {
		return pr, false
	} else if err != nil {
		log.Printf("⚠️  Failed to query pending resolution for user %d: %v", userID, err)
		return pr, false
	}
	return pr, true
}

// AddPendingResolution inserts or replaces a pending resolution in SQLite.
func (s *Storage) AddPendingResolution(userID int64, pr PendingResolution) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO pending_resolutions (user_id, complaint_id, message_id, original_text, prompt_message_id) 
		VALUES (?, ?, ?, ?, ?)
	`, userID, pr.ComplaintNumber, pr.MessageID, pr.OriginalText, pr.PromptMessageID)
	if err != nil {
		log.Printf("⚠️  Failed to save pending resolution for user %d: %v", userID, err)
		return err
	}
	return nil
}

// RemovePendingResolution deletes a pending resolution from SQLite.
func (s *Storage) RemovePendingResolution(userID int64) {
	_, err := s.db.Exec(`DELETE FROM pending_resolutions WHERE user_id = ?`, userID)
	if err != nil {
		log.Printf("⚠️  Failed to delete pending resolution for user %d: %v", userID, err)
	}
}

// Close gracefully closes the SQLite database connection.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// getStorageStats (diagnostic) returns the total rows directly from DB count.
func (s *Storage) getStorageStats() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM complaints`).Scan(&count)
	return count, err
}

func (s *Storage) ensureComplaintColumn(name, typ string) error {
	if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE complaints ADD COLUMN %s %s`, name, typ)); err != nil {
		// Ignore "duplicate column" style errors across SQLite variants.
		if err.Error() != "SQL logic error: duplicate column name: "+name+" (1)" &&
			err.Error() != "duplicate column name: "+name {
			return fmt.Errorf("ensure complaints.%s column: %w", name, err)
		}
	}
	return nil
}
