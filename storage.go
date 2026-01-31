package main

import (
	"encoding/csv"
	"log"
	"os"
	"sync"
)

const complaintFile = "complaints.csv"

type ComplaintStorage struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewComplaintStorage() *ComplaintStorage {
	cs := &ComplaintStorage{
		seen: make(map[string]bool),
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
		if len(record) > 0 {
			cs.seen[record[0]] = true
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

// MarkAsSeen marks a complaint as seen in storage
func (cs *ComplaintStorage) MarkAsSeen(complaintID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.seen[complaintID] = true
}

func (cs *ComplaintStorage) SaveToFile(complaintID string) error {
	file, err := os.OpenFile(complaintFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	return writer.Write([]string{complaintID})
}

func (cs *ComplaintStorage) SaveMultiple(complaintIDs []string) error {
	file, err := os.OpenFile(complaintFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, id := range complaintIDs {
		if err := writer.Write([]string{id}); err != nil {
			return err
		}
	}
	return nil
}
