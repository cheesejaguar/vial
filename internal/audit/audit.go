// Package audit provides an append-only audit log for vault operations.
//
// Every mutation or sensitive read performed by a vial command is recorded as a
// JSON-Lines entry in a log file (default: ~/.local/share/vial/audit.jsonl).
// Entries are written atomically via O_APPEND so concurrent CLI invocations do
// not corrupt the file.  The log is intentionally human-readable: each line is
// a self-contained JSON object ordered newest-first when read back through Log.Read.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// EventType is a string tag that identifies the vault operation being logged.
type EventType string

// Defined event types mirror the alchemical CLI command names wherever possible
// so that an operator reading the log can correlate entries to user actions.
const (
	EventPour    EventType = "pour"    // secrets written into a .env file
	EventBrew    EventType = "brew"    // secrets injected into a subprocess environment
	EventGet     EventType = "get"     // single secret value read
	EventSet     EventType = "set"     // secret created or updated
	EventRemove  EventType = "remove"  // secret deleted from vault
	EventUnlock  EventType = "unlock"  // vault decrypted and session started
	EventLock    EventType = "lock"    // vault session terminated
	EventExport  EventType = "export"  // vault contents exported to a file or stdout
	EventDistill EventType = "distill" // .env file imported into the vault
	EventShare   EventType = "share"   // encrypted bundle created for sharing
)

// Entry is a single audit log record.
// Fields are kept deliberately narrow so the log stays useful without leaking
// secret values — Keys holds key names only, never their values.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`        // UTC time of the operation
	Event     EventType `json:"event"`             // operation type
	Keys      []string  `json:"keys,omitempty"`    // key names involved, if applicable
	Project   string    `json:"project,omitempty"` // project directory path, if applicable
	Detail    string    `json:"detail,omitempty"`  // short human-readable context
}

// Log manages an append-only JSONL audit file at a fixed path.
// The zero value is not usable; create instances with NewLog.
type Log struct {
	path string // absolute path to the .jsonl file
}

// NewLog returns a Log that reads and writes to path.
// The file is created on first Record call; the directory must already exist.
func NewLog(path string) *Log {
	return &Log{path: path}
}

// Record appends a new audit entry to the log file.
// It opens the file with O_APPEND|O_CREATE so concurrent writers cannot
// truncate or interleave partial lines with each other.
func (l *Log) Record(event EventType, keys []string, project, detail string) error {
	entry := Entry{
		Timestamp: time.Now().UTC(),
		Event:     event,
		Keys:      keys,
		Project:   project,
		Detail:    detail,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling audit entry: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()

	// Append the JSON object followed by a newline to form valid JSONL.
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// Read returns audit entries in reverse-chronological order (newest first).
// When limit is positive at most limit entries are returned; pass 0 for no limit.
// A missing log file is treated as an empty log rather than an error, so callers
// do not need to special-case a freshly provisioned vault.
func (l *Log) Read(limit int) ([]Entry, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading audit log: %w", err)
	}

	// Walk the lines in reverse so newer entries are returned first without
	// needing to sort or buffer the entire file.
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entries []Entry

	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err != nil {
			continue // skip malformed entries rather than aborting the read
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}

	return entries, nil
}

// ReadAll returns every audit entry in the log, newest first, with no limit.
// It is a convenience wrapper around Read(0).
func (l *Log) ReadAll() ([]Entry, error) {
	return l.Read(0)
}

// ExportCSV formats a slice of audit entries as a CSV string with a header row.
// Multiple keys in a single entry are joined with semicolons inside the keys
// column so the column count stays constant across all rows.
func ExportCSV(entries []Entry) string {
	var sb strings.Builder
	sb.WriteString("timestamp,event,keys,project,detail\n")
	for _, e := range entries {
		keys := strings.Join(e.Keys, ";")
		sb.WriteString(fmt.Sprintf("%s,%s,%q,%q,%q\n",
			e.Timestamp.Format(time.RFC3339), e.Event, keys, e.Project, e.Detail))
	}
	return sb.String()
}
