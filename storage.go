package main

import (
	"encoding/csv"
	"log"
	"os"
	"sync"
)

const complaintFile = "complaints.csv"

type ComplaintStorage struct {
	mu         sync.Mutex
	seen       map[string]bool
	messageIDs map[string]string // complaintID -> Telegram message ID
}

func NewComplaintStorage() *ComplaintStorage {
	cs := &ComplaintStorage{
		seen:       make(map[string]bool),
		messageIDs: make(map[string]string),
	}
	cs.loadFromFile()
	return cs
}

func (cs *ComplaintStorage) loadFromFile() {
	file, err := os.Open(complaintFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("📋 No existing complaint file found. Creating new one...")
		} else {
			log.Println("⚠️  Failed to open complaint file:", err)
		}
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Println("⚠️  Failed to read complaint file:", err)
		return
	}

	count := 0
	for _, record := range records {
		if len(record) >= 1 {
			cs.seen[record[0]] = true
			if len(record) >= 2 {
				cs.messageIDs[record[0]] = record[1]
			}
			count++
		}
	}
	log.Println("📚 Loaded", count, "previously seen complaints from storage")
}

func (cs *ComplaintStorage) IsNew(complaintID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return !cs.seen[complaintID]
}

func (cs *ComplaintStorage) MarkAsSeen(complaintID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.seen[complaintID] = true
}

func (cs *ComplaintStorage) GetMessageID(complaintID string) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.messageIDs[complaintID]
}

func (cs *ComplaintStorage) SetMessageID(complaintID, messageID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.messageIDs[complaintID] = messageID
}

func (cs *ComplaintStorage) GetAllSeenComplaints() []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	complaints := make([]string, 0, len(cs.seen))
	for id := range cs.seen {
		complaints = append(complaints, id)
	}
	return complaints
}

func (cs *ComplaintStorage) Remove(complaintID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Remove from in-memory maps
	delete(cs.seen, complaintID)
	delete(cs.messageIDs, complaintID)

	// Rewrite CSV file without the removed complaint
	return cs.rewriteFile()
}

func (cs *ComplaintStorage) rewriteFile() error {
	file, err := os.OpenFile(complaintFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for id := range cs.seen {
		record := []string{id}
		if msgID, ok := cs.messageIDs[id]; ok {
			record = append(record, msgID)
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func (cs *ComplaintStorage) SaveToFile(complaintID string, messageID string) error {
	file, err := os.OpenFile(complaintFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	return writer.Write([]string{complaintID, messageID})
}

func (cs *ComplaintStorage) SaveMultiple(complaints []ComplaintRecord) error {
	file, err := os.OpenFile(complaintFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, c := range complaints {
		if err := writer.Write([]string{c.ComplaintID, c.MessageID}); err != nil {
			return err
		}
	}
	return nil
}

type ComplaintRecord struct {
	ComplaintID string
	MessageID   string
}
