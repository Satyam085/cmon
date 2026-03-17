package main

import (
	"os"
	"testing"

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

	stor := storage.New()
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
