package watcher

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/api"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type RouteClient interface {
	UpsertPluginRoute(ctx context.Context, username, pluginID, hostnameHint string, localPort int) (state.Route, error)
	DeletePluginRoute(ctx context.Context, username, routeID string) error
}

type Service struct {
	Username     string
	ConfigPath   string
	SnapshotPath string
	FS           shell.FS
	Routes       RouteClient
	Writer       ConfigWriter
	Now          func() time.Time
}

func DefaultPaths(home string) (configPath, snapshotPath string) {
	return filepath.Join(home, ".openclaw", "openclaw.json"), filepath.Join(home, ".openclaw-overlay", "watcher.state")
}

func CurrentUsername() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

func (s Service) Sync(ctx context.Context) error {
	if s.FS == nil {
		return errors.New("missing filesystem")
	}
	if s.Routes == nil {
		return errors.New("missing route client")
	}
	if s.Writer == nil {
		return errors.New("missing OpenClaw writer")
	}
	username := s.Username
	if username == "" {
		username = CurrentUsername()
	}
	if username == "" {
		return errors.New("missing username")
	}
	data, err := s.FS.ReadFile(s.ConfigPath)
	if err != nil {
		return fmt.Errorf("read OpenClaw config: %w", err)
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return err
	}
	routes, err := DiscoverCallbacks(cfg)
	if err != nil && !errors.Is(err, ErrNoCallbackRoutes) {
		return err
	}
	snap, err := LoadSnapshot(s.FS, s.SnapshotPath)
	if err != nil {
		return err
	}
	diff := DiffSnapshot(snap, routes)
	for _, removed := range diff.Removed {
		if removed.RouteID == "" {
			continue
		}
		if err := s.Routes.DeletePluginRoute(ctx, username, removed.RouteID); err != nil {
			return fmt.Errorf("delete plugin route %q: %w", removed.RouteID, err)
		}
		delete(snap.Plugins, SnapshotKey(CallbackRoute{PluginID: removed.PluginID, HostnameHint: removed.HostnameHint}))
	}
	for _, route := range diff.AddedOrUpdated {
		published, err := s.Routes.UpsertPluginRoute(ctx, username, route.PluginID, route.HostnameHint, route.LocalPort)
		if err != nil {
			return fmt.Errorf("publish plugin route %q: %w", route.PluginID, err)
		}
		if err := s.Writer.SetCallback(ctx, route, "https://"+published.Hostname, published.LocalPort); err != nil {
			return err
		}
		snap.Plugins[SnapshotKey(route)] = SnapshotEntry{
			PluginID:      route.PluginID,
			HostnameHint:  route.HostnameHint,
			LocalPort:     route.LocalPort,
			RouteID:       published.ID,
			URL:           "https://" + published.Hostname,
			URLConfigKey:  route.URLConfigKey,
			PortConfigKey: route.PortConfigKey,
			Synced:        true,
			UpdatedAt:     s.now(),
		}
	}
	return SaveSnapshot(s.FS, s.SnapshotPath, snap)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func NewAPIClient(socketPath string) api.Client {
	return api.Client{SocketPath: socketPath}
}
