package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

type fakeSource struct {
	events chan string
	errs   chan error
}

func (f fakeSource) Events() <-chan string { return f.events }
func (f fakeSource) Errors() <-chan error  { return f.errs }
func (f fakeSource) Close() error          { return nil }

func TestLoopDebouncesEvents(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/home/alice/.openclaw/openclaw.json"] = []byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":19000}}}}}`)
	routes := &fakeRoutes{}
	writer := &fakeWriter{}
	source := fakeSource{events: make(chan string, 4), errs: make(chan error, 1)}
	svc := Service{Username: "alice", ConfigPath: "/home/alice/.openclaw/openclaw.json", SnapshotPath: "/state", FS: fs, Routes: routes, Writer: writer}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Loop{Service: svc, Source: source, ConfigPath: svc.ConfigPath, Debounce: 10 * time.Millisecond}.Run(ctx)
	}()
	source.events <- "/home/alice/.openclaw/openclaw.json"
	source.events <- "/home/alice/.openclaw/openclaw.json"
	time.Sleep(40 * time.Millisecond)
	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if len(routes.upserts) != 1 {
		t.Fatalf("expected one debounced upsert after initial sync made snapshot current, got %d", len(routes.upserts))
	}
}

func TestLoopHandlesSourceErrors(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/cfg"] = []byte(`{}`)
	source := fakeSource{events: make(chan string), errs: make(chan error, 1)}
	var handled error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		source.errs <- errors.New("watch failed")
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := Loop{
		Service: Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state", FS: fs, Routes: &fakeRoutes{}, Writer: &fakeWriter{}},
		Source:  source,
		ErrorHandler: func(err error) {
			handled = err
		},
	}.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if handled == nil || handled.Error() != "watch failed" {
		t.Fatalf("unexpected handled error: %v", handled)
	}
}

func TestFSNotifySourceReceivesFileEvent(t *testing.T) {
	dir := t.TempDir()
	source, err := NewFSNotifySource(dir)
	if err != nil {
		t.Fatalf("NewFSNotifySource: %v", err)
	}
	defer func() { _ = source.Close() }()
	target := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	select {
	case got := <-source.Events():
		if filepath.Base(got) != "openclaw.json" {
			t.Fatalf("unexpected event path: %s", got)
		}
	case err := <-source.Errors():
		t.Fatalf("watch error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fsnotify event")
	}
}
