package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// EventType represents the type of audit event.
type EventType string

const (
	EventPour    EventType = "pour"
	EventBrew    EventType = "brew"
	EventGet     EventType = "get"
	EventSet     EventType = "set"
	EventRemove  EventType = "remove"
	EventUnlock  EventType = "unlock"
	EventLock    EventType = "lock"
	EventExport  EventType = "export"
	EventDistill EventType = "distill"
	EventShare   EventType = "share"
)

// Entry is a single audit log entry.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Event     EventType `json:"event"`
	Keys      []string  `json:"keys,omitempty"`
	Project   string    `json:"project,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// Log manages the audit log file.
type Log struct {
	path string
}

// NewLog creates a new audit log at the given path.
func NewLog(path string) *Log {
	return &Log{path: path}
}

// Record writes an audit entry to the log.
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

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// Read returns all audit entries, newest first.
func (l *Log) Read(limit int) ([]Entry, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading audit log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entries []Entry

	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}

	return entries, nil
}

// ReadAll returns all audit entries (newest first, no limit).
func (l *Log) ReadAll() ([]Entry, error) {
	return l.Read(0)
}

// ExportCSV writes audit entries in CSV format to the given writer.
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
