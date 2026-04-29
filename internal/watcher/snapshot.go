package watcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

type Snapshot struct {
	Plugins map[string]SnapshotEntry `json:"plugins"`
}

type SnapshotEntry struct {
	PluginID      string    `json:"plugin_id"`
	HostnameHint  string    `json:"hostname_hint"`
	LocalPort     int       `json:"local_port"`
	RouteID       string    `json:"route_id"`
	URL           string    `json:"url"`
	URLConfigKey  string    `json:"url_config_key"`
	PortConfigKey string    `json:"port_config_key"`
	Synced        bool      `json:"synced"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func LoadSnapshot(fs shell.FS, path string) (Snapshot, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{Plugins: map[string]SnapshotEntry{}}, nil
		}
		return Snapshot{}, fmt.Errorf("read watcher snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("parse watcher snapshot: %w", err)
	}
	if snap.Plugins == nil {
		snap.Plugins = map[string]SnapshotEntry{}
	}
	return snap, nil
}

func SaveSnapshot(fs shell.FS, path string, snap Snapshot) error {
	if snap.Plugins == nil {
		snap.Plugins = map[string]SnapshotEntry{}
	}
	if err := fs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir watcher snapshot dir: %w", err)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal watcher snapshot: %w", err)
	}
	if err := fs.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write watcher snapshot: %w", err)
	}
	return nil
}

func SnapshotKey(route CallbackRoute) string {
	return route.PluginID + "\x00" + route.HostnameHint
}

type Diff struct {
	AddedOrUpdated []CallbackRoute
	Removed        []SnapshotEntry
	Unchanged      []SnapshotEntry
}

func DiffSnapshot(snap Snapshot, routes []CallbackRoute) Diff {
	current := make(map[string]CallbackRoute, len(routes))
	for _, route := range routes {
		current[SnapshotKey(route)] = route
	}
	var diff Diff
	for key, route := range current {
		prev, ok := snap.Plugins[key]
		if !ok || prev.LocalPort != route.LocalPort || prev.URLConfigKey != route.URLConfigKey || prev.PortConfigKey != route.PortConfigKey || !prev.Synced {
			diff.AddedOrUpdated = append(diff.AddedOrUpdated, route)
			continue
		}
		diff.Unchanged = append(diff.Unchanged, prev)
	}
	for key, prev := range snap.Plugins {
		if _, ok := current[key]; !ok {
			diff.Removed = append(diff.Removed, prev)
		}
	}
	return diff
}
