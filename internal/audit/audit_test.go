package audit

import (
	"path/filepath"
	"testing"
	"time"
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
	// The limit keeps the newest entries.
	if limited[0].Event != EventGet || limited[1].Event != EventPour {
		t.Errorf("limited events = [%s %s], want [get pour]", limited[0].Event, limited[1].Event)
	}
	if got := entries[1].Keys; len(got) != 2 || got[0] != "OPENAI_API_KEY" || got[1] != "DB_URL" {
		t.Errorf("pour keys = %v, want [OPENAI_API_KEY DB_URL]", got)
	}
	if entries[1].Project != "/tmp/project" || entries[1].Detail != "2 matched" {
		t.Errorf("pour project/detail = %q/%q", entries[1].Project, entries[1].Detail)
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

// TestExportCSV checks the exact CSV output: header row, one row per entry,
// RFC 3339 timestamps, keys joined with ";", and quoted text columns.
func TestExportCSV(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entries := []Entry{
		{Timestamp: ts, Event: EventPour, Keys: []string{"KEY1", "KEY2"}, Project: "/tmp/p", Detail: "2 matched"},
		{Timestamp: ts, Event: EventSet, Keys: []string{"KEY3"}},
	}

	want := "timestamp,event,keys,project,detail\n" +
		`2026-01-02T03:04:05Z,pour,"KEY1;KEY2","/tmp/p","2 matched"` + "\n" +
		`2026-01-02T03:04:05Z,set,"KEY3","",""` + "\n"
	if got := ExportCSV(entries); got != want {
		t.Errorf("ExportCSV =\n%s\nwant\n%s", got, want)
	}
}
