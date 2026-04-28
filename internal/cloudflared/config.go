package cloudflared

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

const DefaultConfigPath = "/etc/cloudflared/config.yml"

func CredentialsFile(cfg *config.OverlayConfig) (string, error) {
	if cfg == nil {
		return "", errors.New("missing overlay config")
	}
	if strings.TrimSpace(cfg.CloudflaredCredentialsFile) != "" {
		return strings.TrimSpace(cfg.CloudflaredCredentialsFile), nil
	}
	tunnelID := strings.TrimSpace(cfg.TunnelID)
	if tunnelID == "" {
		return "", errors.New("missing tunnel_id")
	}
	return "/etc/cloudflared/" + tunnelID + ".json", nil
}

func RenderConfig(cfg *config.OverlayConfig, routes []state.Route) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("missing overlay config")
	}
	tunnelID := strings.TrimSpace(cfg.TunnelID)
	if tunnelID == "" {
		return nil, errors.New("missing tunnel_id")
	}
	credentials, err := CredentialsFile(cfg)
	if err != nil {
		return nil, err
	}
	enabled := make([]state.Route, 0, len(routes))
	for _, r := range routes {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		a, b := enabled[i], enabled[j]
		if a.Hostname != b.Hostname {
			return a.Hostname < b.Hostname
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.PluginID != b.PluginID {
			return a.PluginID < b.PluginID
		}
		return a.ID < b.ID
	})
	var b strings.Builder
	fmt.Fprintf(&b, "tunnel: %s\n", tunnelID)
	fmt.Fprintf(&b, "credentials-file: %s\n\n", credentials)
	b.WriteString("ingress:\n")
	for _, r := range enabled {
		if strings.TrimSpace(r.Hostname) == "" {
			return nil, fmt.Errorf("route %q has empty hostname", r.ID)
		}
		if r.LocalPort <= 0 {
			return nil, fmt.Errorf("route %q has invalid local port", r.ID)
		}
		fmt.Fprintf(&b, "  - hostname: %s\n", r.Hostname)
		fmt.Fprintf(&b, "    service: http://127.0.0.1:%d\n", r.LocalPort)
	}
	b.WriteString("  - service: http_status:404\n")
	return []byte(b.String()), nil
}
