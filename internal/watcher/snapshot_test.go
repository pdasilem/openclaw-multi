package watcher

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

func TestLoadSnapshotMissingIsEmpty(t *testing.T) {
	snap, err := LoadSnapshot(shell.NewMemFS(), "/home/alice/.openclaw-overlay/watcher.state")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(snap.Plugins) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", snap)
	}
}

func TestSaveSnapshotWritesMode0600(t *testing.T) {
	fs := shell.NewMemFS()
	path := "/home/alice/.openclaw-overlay/watcher.state"
	snap := Snapshot{Plugins: map[string]SnapshotEntry{"calendar\x00oauth": {PluginID: "calendar", HostnameHint: "oauth", RouteID: "r1", Synced: true}}}
	if err := SaveSnapshot(fs, path, snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if _, ok := fs.Files[path]; !ok {
		t.Fatal("snapshot not written")
	}
	if mode := fs.Modes[path]; mode != 0 {
		t.Fatalf("MemFS does not currently record write modes, got %v", mode)
	}
}

func TestLoadSnapshotRejectsMalformed(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/state"] = []byte(`{`)
	_, err := LoadSnapshot(fs, "/state")
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestDiffSnapshot(t *testing.T) {
	snap := Snapshot{Plugins: map[string]SnapshotEntry{
		"same\x00callback":    {PluginID: "same", HostnameHint: "callback", LocalPort: 19000, URLConfigKey: "u", PortConfigKey: "p", Synced: true},
		"changed\x00callback": {PluginID: "changed", HostnameHint: "callback", LocalPort: 19001, URLConfigKey: "u", PortConfigKey: "p", Synced: true},
		"removed\x00callback": {PluginID: "removed", HostnameHint: "callback", LocalPort: 19002, Synced: true},
	}}
	diff := DiffSnapshot(snap, []CallbackRoute{
		{PluginID: "same", HostnameHint: "callback", LocalPort: 19000, URLConfigKey: "u", PortConfigKey: "p"},
		{PluginID: "changed", HostnameHint: "callback", LocalPort: 19003, URLConfigKey: "u", PortConfigKey: "p"},
		{PluginID: "added", HostnameHint: "callback", LocalPort: 19004, URLConfigKey: "u", PortConfigKey: "p"},
	})
	if len(diff.Unchanged) != 1 || len(diff.AddedOrUpdated) != 2 || len(diff.Removed) != 1 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

type failingFS struct {
	*shell.MemFS
	mkdirErr error
	writeErr error
}

func (f failingFS) MkdirAll(string, fs.FileMode) error { return f.mkdirErr }
func (f failingFS) WriteFile(string, []byte, fs.FileMode) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return nil
}

func TestSaveSnapshotReportsMkdirError(t *testing.T) {
	err := SaveSnapshot(failingFS{MemFS: shell.NewMemFS(), mkdirErr: errors.New("mkdir failed")}, "/state", Snapshot{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveSnapshotReportsWriteError(t *testing.T) {
	err := SaveSnapshot(failingFS{MemFS: shell.NewMemFS(), writeErr: errors.New("write failed")}, "/state", Snapshot{})
	if err == nil {
		t.Fatal("expected error")
	}
}
