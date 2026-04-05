package audit

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRecordAndRead verifies that events are persisted and returned newest-first,
// and that the limit parameter correctly caps the result set.
func TestRecordAndRead(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")
	log := NewLog(logPath)

	// Record some events
	if err := log.Record(EventSet, []string{"OPENAI_API_KEY"}, "", "stored"); err != nil {
		t.Fatal(err)
	}
	if err := log.Record(EventPour, []string{"OPENAI_API_KEY", "DB_URL"}, "/tmp/project", "2 matched"); err != nil {
		t.Fatal(err)
	}
	if err := log.Record(EventGet, []string{"DB_URL"}, "", ""); err != nil {
		t.Fatal(err)
	}

	// Read all
	entries, err := log.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Newest first
	if entries[0].Event != EventGet {
		t.Errorf("expected newest entry to be 'get', got %q", entries[0].Event)
	}
	if entries[1].Event != EventPour {
		t.Errorf("expected second entry to be 'pour', got %q", entries[1].Event)
	}

	// Read with limit
	limited, err := log.Read(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 entries with limit, got %d", len(limited))
	}
}

// TestReadEmpty confirms that reading a non-existent log file returns an empty
// slice rather than an error, matching fresh-vault behaviour.
func TestReadEmpty(t *testing.T) {
	dir := t.TempDir()
	log := NewLog(filepath.Join(dir, "nonexistent.jsonl"))

	entries, err := log.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestExportCSV checks that the CSV output includes the header row and at least
// one event name from the provided entries.
func TestExportCSV(t *testing.T) {
	entries := []Entry{
		{Event: EventPour, Keys: []string{"KEY1", "KEY2"}, Project: "/tmp/p"},
		{Event: EventSet, Keys: []string{"KEY3"}},
	}

	csv := ExportCSV(entries)
	if !strings.Contains(csv, "timestamp,event,keys,project,detail") {
		t.Error("missing CSV header")
	}
	if !strings.Contains(csv, "pour") {
		t.Error("missing pour entry")
	}
}
