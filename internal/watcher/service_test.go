package watcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type fakeRoutes struct {
	upserts []CallbackRoute
	deletes []string
	err     error
}

func (f *fakeRoutes) UpsertPluginRoute(_ context.Context, username, pluginID, hostnameHint string, localPort int) (state.Route, error) {
	if f.err != nil {
		return state.Route{}, f.err
	}
	route := CallbackRoute{PluginID: pluginID, HostnameHint: hostnameHint, LocalPort: localPort}
	f.upserts = append(f.upserts, route)
	return state.Route{ID: "plugin:" + username + ":" + pluginID + ":" + hostnameHint, Username: username, Kind: state.RouteKindPlugin, PluginID: pluginID, Hostname: hostnameHint + ".example.com", LocalPort: localPort, Enabled: true}, nil
}

func (f *fakeRoutes) DeletePluginRoute(_ context.Context, _ string, routeID string) error {
	f.deletes = append(f.deletes, routeID)
	return f.err
}

type fakeWriter struct {
	writes []string
	err    error
}

func (f *fakeWriter) SetCallback(_ context.Context, route CallbackRoute, url string, localPort int) error {
	if f.err != nil {
		return f.err
	}
	f.writes = append(f.writes, route.PluginID+"="+url+":"+time.Duration(localPort).String())
	return nil
}

func TestServiceSyncPublishesAndWritesSnapshot(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/home/alice/.openclaw/openclaw.json"] = []byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":19000,"callbackHostnameHint":"oauth"}}}}}`)
	routes := &fakeRoutes{}
	writer := &fakeWriter{}
	svc := Service{Username: "alice", ConfigPath: "/home/alice/.openclaw/openclaw.json", SnapshotPath: "/home/alice/.openclaw-overlay/watcher.state", FS: fs, Routes: routes, Writer: writer, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	if err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(routes.upserts) != 1 || routes.upserts[0].PluginID != "calendar" {
		t.Fatalf("unexpected upserts: %+v", routes.upserts)
	}
	if len(writer.writes) != 1 || !strings.Contains(writer.writes[0], "https://oauth.example.com") {
		t.Fatalf("unexpected writes: %+v", writer.writes)
	}
	if _, ok := fs.Files["/home/alice/.openclaw-overlay/watcher.state"]; !ok {
		t.Fatal("snapshot not written")
	}
}

func TestServiceSyncIsIdempotent(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/cfg"] = []byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":19000,"callbackHostnameHint":"oauth"}}}}}`)
	fs.Files["/state"] = []byte(`{"plugins":{"calendar\u0000oauth":{"plugin_id":"calendar","hostname_hint":"oauth","local_port":19000,"route_id":"r1","url":"https://oauth.example.com","url_config_key":"plugins.entries.calendar.config.callbackUrl","port_config_key":"plugins.entries.calendar.config.callbackPort","synced":true}}}`)
	routes := &fakeRoutes{}
	writer := &fakeWriter{}
	svc := Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state", FS: fs, Routes: routes, Writer: writer}
	if err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(routes.upserts) != 0 || len(writer.writes) != 0 {
		t.Fatalf("expected idempotent no-op, upserts=%+v writes=%+v", routes.upserts, writer.writes)
	}
}

func TestServiceSyncDeletesRemovedPlugin(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/cfg"] = []byte(`{"plugins":{"entries":{}}}`)
	fs.Files["/state"] = []byte(`{"plugins":{"calendar\u0000oauth":{"plugin_id":"calendar","hostname_hint":"oauth","local_port":19000,"route_id":"r1","synced":true}}}`)
	routes := &fakeRoutes{}
	writer := &fakeWriter{}
	svc := Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state", FS: fs, Routes: routes, Writer: writer}
	if err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(routes.deletes) != 1 || routes.deletes[0] != "r1" {
		t.Fatalf("unexpected deletes: %+v", routes.deletes)
	}
}

func TestServiceSyncDoesNotWriteConfigOnAPIError(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/cfg"] = []byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":19000}}}}}`)
	routes := &fakeRoutes{err: errors.New("api down")}
	writer := &fakeWriter{}
	svc := Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state", FS: fs, Routes: routes, Writer: writer}
	if err := svc.Sync(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if len(writer.writes) != 0 {
		t.Fatalf("unexpected writes: %+v", writer.writes)
	}
}

func TestServiceSyncDoesNotMarkSyncedOnWriterError(t *testing.T) {
	fs := shell.NewMemFS()
	fs.Files["/cfg"] = []byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":19000}}}}}`)
	routes := &fakeRoutes{}
	writer := &fakeWriter{err: errors.New("write failed")}
	svc := Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state", FS: fs, Routes: routes, Writer: writer}
	if err := svc.Sync(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if _, ok := fs.Files["/state"]; ok {
		t.Fatal("snapshot should not be written")
	}
}

func TestDefaultPaths(t *testing.T) {
	configPath, snapshotPath := DefaultPaths("/home/alice")
	if configPath != "/home/alice/.openclaw/openclaw.json" {
		t.Fatalf("unexpected config path: %s", configPath)
	}
	if snapshotPath != "/home/alice/.openclaw-overlay/watcher.state" {
		t.Fatalf("unexpected snapshot path: %s", snapshotPath)
	}
}

func TestNewAPIClient(t *testing.T) {
	client := NewAPIClient("/tmp/socket")
	if client.SocketPath != "/tmp/socket" {
		t.Fatalf("unexpected socket: %s", client.SocketPath)
	}
}

func TestServiceSyncValidatesDependencies(t *testing.T) {
	base := Service{Username: "alice", ConfigPath: "/cfg", SnapshotPath: "/state"}
	if err := base.Sync(context.Background()); err == nil {
		t.Fatal("expected missing filesystem error")
	}
	fsOnly := base
	fsOnly.FS = shell.NewMemFS()
	if err := fsOnly.Sync(context.Background()); err == nil {
		t.Fatal("expected missing route client error")
	}
	routesOnly := fsOnly
	routesOnly.Routes = &fakeRoutes{}
	if err := routesOnly.Sync(context.Background()); err == nil {
		t.Fatal("expected missing writer error")
	}
}
