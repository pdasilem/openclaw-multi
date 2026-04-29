// Package watcher implements the per-user OpenClaw overlay watcher.
package watcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrNoCallbackRoutes = errors.New("no callback routes")

type Config struct {
	Plugins PluginConfig `json:"plugins"`
}

type PluginConfig struct {
	Entries map[string]PluginEntry `json:"entries"`
}

type PluginEntry struct {
	Config  map[string]any `json:"config"`
	Overlay OverlayConfig  `json:"overlay"`
}

type OverlayConfig struct {
	Callback CallbackConfig `json:"callback"`
}

type CallbackConfig struct {
	Enabled         bool   `json:"enabled"`
	LocalPort       int    `json:"local_port"`
	HostnameHint    string `json:"hostname_hint"`
	URLConfigKey    string `json:"url_config_key"`
	PortConfigKey   string `json:"port_config_key"`
	LegacyLocalPort int    `json:"callbackPort"`
}

type CallbackRoute struct {
	PluginID      string
	HostnameHint  string
	LocalPort     int
	URLConfigKey  string
	PortConfigKey string
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse openclaw config: %w", err)
	}
	if cfg.Plugins.Entries == nil {
		cfg.Plugins.Entries = map[string]PluginEntry{}
	}
	return cfg, nil
}

func DiscoverCallbacks(cfg Config) ([]CallbackRoute, error) {
	routes := make([]CallbackRoute, 0, len(cfg.Plugins.Entries))
	for pluginID, entry := range cfg.Plugins.Entries {
		route, ok, err := callbackRoute(pluginID, entry)
		if err != nil {
			return nil, err
		}
		if ok {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		return nil, ErrNoCallbackRoutes
	}
	return routes, nil
}

func callbackRoute(pluginID string, entry PluginEntry) (CallbackRoute, bool, error) {
	callback := entry.Overlay.Callback
	port := callback.LocalPort
	if port == 0 {
		port = callback.LegacyLocalPort
	}
	if port == 0 {
		port = intFromConfig(entry.Config, "callbackPort")
	}
	if port == 0 {
		return CallbackRoute{}, false, nil
	}
	if port < 1 || port > 65535 {
		return CallbackRoute{}, false, fmt.Errorf("plugin %q callback port out of range: %d", pluginID, port)
	}
	hostnameHint := strings.TrimSpace(callback.HostnameHint)
	if hostnameHint == "" {
		hostnameHint = stringFromConfig(entry.Config, "callbackHostnameHint")
	}
	if hostnameHint == "" {
		hostnameHint = "callback"
	}
	return CallbackRoute{
		PluginID:      pluginID,
		HostnameHint:  hostnameHint,
		LocalPort:     port,
		URLConfigKey:  defaultString(callback.URLConfigKey, "plugins.entries."+pluginID+".config.callbackUrl"),
		PortConfigKey: defaultString(callback.PortConfigKey, "plugins.entries."+pluginID+".config.callbackPort"),
	}, true, nil
}

func intFromConfig(values map[string]any, key string) int {
	switch v := values[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return 0
}

func stringFromConfig(values map[string]any, key string) string {
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func defaultString(value, defaultValue string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return defaultValue
}
