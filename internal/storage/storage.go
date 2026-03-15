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
	Village      string
	Belt         string
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
	villages             map[string]string // complaintID → village
	belts                map[string]string // complaintID → belt
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
func New() *Storage {
	s := &Storage{
		seen:                 make(map[string]bool),
		messageIDs:           make(map[string]string),
		waMessageIDs:         make(map[string]string),
		waMessageToComplaint: make(map[string]string),
		apiIDs:               make(map[string]string),
		consumerNames:        make(map[string]string),
		villages:             make(map[string]string),
		belts:                make(map[string]string),
	}

	// Connect to SQLite
	db, err := sql.Open("sqlite", dbFile+"?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database %s: %v", dbFile, err)
	}

	importTime := time.Now()
	_ = importTime // for time package use

	// Configure connection pooling
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	s.db = db

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

	s.ensureComplaintColumn("village", "TEXT")
	s.ensureComplaintColumn("belt", "TEXT")

	// Run migration from old complaints.csv if needed
	s.migrateFromCSV()

	// Load data from DB into memory maps
	s.loadFromDB()

	return s
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
	rows, err := s.db.Query(`SELECT complaint_id, tg_message_id, wa_message_id, api_id, consumer_name, village, belt FROM complaints`)
	if err != nil {
		log.Fatalf("❌ Failed to query database on load: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var complaintID, tgMessageID, waMessageID, apiID, consumerName, village, belt sql.NullString
		if err := rows.Scan(&complaintID, &tgMessageID, &waMessageID, &apiID, &consumerName, &village, &belt); err != nil {
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
			if village.Valid {
				s.villages[complaintID.String] = village.String
			}
			if belt.Valid {
				s.belts[complaintID.String] = belt.String
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

// SetMessageID sets the Telegram message ID for a complaint (memory only).
func (s *Storage) SetMessageID(complaintID, messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageIDs[complaintID] = messageID
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

	// Update memory
	s.waMessageIDs[complaintID] = waMessageID
	s.waMessageToComplaint[waMessageID] = complaintID

	// Update DB immediately
	_, err := s.db.Exec(`UPDATE complaints SET wa_message_id = ? WHERE complaint_id = ?`, waMessageID, complaintID)
	if err != nil {
		log.Printf("⚠️  Failed to persist WA message ID for %s: %v", complaintID, err)
		return err
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

// GetVillage retrieves the stored village for a complaint.
func (s *Storage) GetVillage(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.villages[complaintID]
}

// GetBelt retrieves the stored belt for a complaint.
func (s *Storage) GetBelt(complaintID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.belts[complaintID]
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
		INSERT INTO complaints (complaint_id, tg_message_id, wa_message_id, api_id, consumer_name, village, belt)
		VALUES (?, ?, ?, ?, ?, ?, ?)
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
			village = CASE
				WHEN excluded.village != '' THEN excluded.village
				ELSE complaints.village
			END,
			belt = CASE
				WHEN excluded.belt != '' THEN excluded.belt
				ELSE complaints.belt
			END
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		if _, err := stmt.Exec(r.ComplaintID, r.MessageID, r.WAMessageID, r.APIID, r.ConsumerName, r.Village, r.Belt); err != nil {
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
			s.waMessageIDs[r.ComplaintID] = r.WAMessageID
			s.waMessageToComplaint[r.WAMessageID] = r.ComplaintID
		}
		if r.APIID != "" {
			s.apiIDs[r.ComplaintID] = r.APIID
		}
		if r.ConsumerName != "" {
			s.consumerNames[r.ComplaintID] = r.ConsumerName
		}
		if r.Village != "" {
			s.villages[r.ComplaintID] = r.Village
		}
		if r.Belt != "" {
			s.belts[r.ComplaintID] = r.Belt
		}
	}

	return nil
}

// Remove permanently deletes a complaint from SQLite and memory.
func (s *Storage) Remove(complaintID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM complaints WHERE complaint_id = ?`, complaintID)
	if err != nil {
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
	delete(s.villages, complaintID)
	delete(s.belts, complaintID)

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

	_, err := s.db.Exec(`DELETE FROM complaints WHERE complaint_id = ?`, complaintID)
	if err != nil {
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
	delete(s.villages, complaintID)
	delete(s.belts, complaintID)

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

func (s *Storage) ensureComplaintColumn(name, typ string) {
	if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE complaints ADD COLUMN %s %s`, name, typ)); err != nil {
		// Ignore "duplicate column" style errors across SQLite variants.
		if err.Error() != "SQL logic error: duplicate column name: "+name+" (1)" &&
			err.Error() != "duplicate column name: "+name {
			log.Fatalf("❌ Failed to ensure complaints.%s column: %v", name, err)
		}
	}
}
