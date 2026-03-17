package storage

import (
	"os"
	"testing"
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

	stor := New()
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

	stor := New()
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

func TestRemoveDeletesPendingResolutions(t *testing.T) {
	withTempCWD(t)

	stor := New()
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
