// Package config handles overlay configuration file I/O and template rendering.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned by Load when the config file does not exist.
var ErrNotFound = errors.New("config file not found")

// OverlayConfig is the structure of /etc/openclaw-multi/config.yml.
type OverlayConfig struct {
	Domain                     string        `yaml:"domain"`
	Subdomain                  string        `yaml:"subdomain"`
	TunnelName                 string        `yaml:"tunnel_name"`
	TunnelID                   string        `yaml:"tunnel_id"`
	TunnelMode                 string        `yaml:"tunnel_mode"` // "account" | "quick"
	CloudflareZoneID           string        `yaml:"cloudflare_zone_id"`
	CloudflareAPIToken         string        `yaml:"cloudflare_api_token"`
	CloudflaredCredentialsFile string        `yaml:"cloudflared_credentials_file"`
	PortRangeStart             int           `yaml:"port_range_start"`
	PortRangeStep              int           `yaml:"port_range_step"`
	NodeVersionMin             string        `yaml:"node_version_min"`
	TerminalHistoryLines       int           `yaml:"terminal_history_lines"`
	Notifications              Notifications `yaml:"notifications"`
}

// Notifications holds Telegram bot credentials for admin alerts.
type Notifications struct {
	TelegramToken  string `yaml:"telegram_token"`
	TelegramChatID string `yaml:"telegram_chat_id"`
}

// Defaults returns an OverlayConfig pre-filled with sensible defaults.
func Defaults() *OverlayConfig {
	return &OverlayConfig{
		Subdomain:            "openclaw",
		TunnelName:           "openclaw-multi",
		TunnelMode:           "account",
		PortRangeStart:       18789,
		PortRangeStep:        20,
		NodeVersionMin:       "24",
		TerminalHistoryLines: 1000,
	}
}

// Load reads the config from path. If the file does not exist, returns
// (Defaults(), ErrNotFound). Other I/O errors are returned as-is.
func Load(path string) (*OverlayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), ErrNotFound
		}
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	cfg := Defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path atomically (temp file + rename).
func Save(path string, cfg *OverlayConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp config %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config %q → %q: %w", tmp, path, err)
	}
	return nil
}
