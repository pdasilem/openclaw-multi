package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const defaultLogPath = "/var/log/openclaw-multi/audit.log"

// Event is a single audit log entry.
type Event struct {
	TS           time.Time      `json:"ts"`
	Actor        string         `json:"actor"`
	Action       ActionType     `json:"action"`
	Target       string         `json:"target"`
	Result       Result         `json:"result"`
	Details      map[string]any `json:"details,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
}

// Logger writes JSONL audit entries to a file.
type Logger struct {
	f *os.File
}

// New opens (or creates) the audit log at path for append-only writing.
// If path is empty, defaultLogPath is used.
// The file is created with mode 0600.
func New(path string) (*Logger, error) {
	if path == "" {
		path = defaultLogPath
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log %q: %w", path, err)
	}
	return &Logger{f: f}, nil
}

// Emit writes one JSONL line for event e.
func (l *Logger) Emit(e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	b = append(b, '\n')
	if _, err := l.f.Write(b); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (l *Logger) Close() error {
	return l.f.Close()
}
