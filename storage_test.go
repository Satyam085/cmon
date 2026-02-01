package main

import (
	"os"
	"testing"
)

func TestComplaintStorage_IsNew(t *testing.T) {
	storage := NewComplaintStorage()

	// New complaint should return true
	if !storage.IsNew("CP001") {
		t.Error("expected IsNew to return true for new complaint")
	}

	// Mark as seen
	storage.MarkAsSeen("CP001")

	// After marking as seen, should return false
	if storage.IsNew("CP001") {
		t.Error("expected IsNew to return false for seen complaint")
	}
}

func TestComplaintStorage_MarkAsSeen(t *testing.T) {
	storage := NewComplaintStorage()

	complaintID := "CP002"

	// Initially should be new
	if !storage.IsNew(complaintID) {
		t.Error("expected complaint to be new initially")
	}

	// Mark as seen
	storage.MarkAsSeen(complaintID)

	// Should no longer be new
	if storage.IsNew(complaintID) {
		t.Error("expect complaint to not be new after MarkAsSeen")
	}
}

func TestComplaintStorage_SaveMultiple(t *testing.T) {
	// Create temp file for testing
	tmpFile := "test_complaints.csv"
	defer os.Remove(tmpFile)

	// Temporarily change the complaint file constant
	// Note: This is a limitation of the current design -
	// ideally complaintFile should be configurable

	storage := NewComplaintStorage()

	complaints := []ComplaintRecord{
		{ComplaintID: "CP001", MessageID: "msg1"},
		{ComplaintID: "CP002", MessageID: "msg2"},
		{ComplaintID: "CP003", MessageID: "msg3"},
	}
	err := storage.SaveMultiple(complaints)

	if err != nil {
		t.Errorf("expected no error saving complaints but got: %v", err)
	}
}

func TestComplaintStorage_Concurrency(t *testing.T) {
	storage := NewComplaintStorage()

	// Test concurrent access
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			complaintID := "CPSendMessage" + string(rune('0'+id))
			storage.MarkAsSeen(complaintID)
			storage.IsNew(complaintID)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock, the mutex is working
}
