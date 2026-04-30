package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestLogger(t *testing.T) (*Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	l, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return l, path
}

func TestNewCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "audit.log")
	l, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat audit log: %v", err)
	}
}

func TestNewCreatesFileWith0600(t *testing.T) {
	_, path := newTestLogger(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}
}

func TestEmitWritesValidJSONL(t *testing.T) {
	l, path := newTestLogger(t)

	events := []Event{
		{Actor: "opadmin", Action: ActionStartup, Target: "system", Result: ResultOk},
		{Actor: "opadmin", Action: ActionAdminSet, Target: "opadmin", Result: ResultOk},
		{Actor: "opadmin", Action: ActionBootstrapUser, Target: "alice", Result: ResultOk,
			Details: map[string]any{"port": 18789}},
	}
	for _, e := range events {
		if err := l.Emit(e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	var count int
	for sc.Scan() {
		line := sc.Bytes()
		var v map[string]any
		if err := json.Unmarshal(line, &v); err != nil {
			t.Errorf("line %d not valid JSON: %v", count+1, err)
		}
		for _, field := range []string{"ts", "actor", "action", "target", "result"} {
			if _, ok := v[field]; !ok {
				t.Errorf("line %d missing field %q", count+1, field)
			}
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 lines, got %d", count)
	}
}

func TestEmitAppendsNotOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	emit := func(n int) {
		l, err := New(path)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = l.Close() }()
		for i := 0; i < n; i++ {
			if err := l.Emit(Event{
				TS:     time.Now().UTC(),
				Actor:  "opadmin",
				Action: ActionStartup,
				Target: "system",
				Result: ResultOk,
			}); err != nil {
				t.Fatalf("Emit: %v", err)
			}
		}
	}

	emit(3)
	emit(2)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	var count int
	for sc.Scan() {
		count++
	}
	if count != 5 {
		t.Errorf("expected 5 lines (3+2), got %d", count)
	}
}
