package deps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// TunnelMode selects the cloudflared installation variant.
const (
	TunnelModeAccount = "account" // Named tunnel with Cloudflare account (Variant A)
	TunnelModeQuick   = "quick"   // Quick tunnel without account (Variant B)
)

// ConflictPolicy controls behaviour when an existing cloudflared config is found.
type ConflictPolicy int

const (
	ConflictBackup    ConflictPolicy = iota // Backup existing file and overwrite
	ConflictSkip                            // Keep existing, do nothing
	ConflictOverwrite                       // Overwrite without backup
)

const (
	cfCredPath    = "/etc/cloudflared"
	cfConfigPath  = "/etc/cloudflared/config.yml"
	cfServiceName = "cloudflared"
)

// EnsureCloudflared installs and configures cloudflared idempotently.
func EnsureCloudflared(
	ctx context.Context,
	exec shell.Executor,
	fs shell.FS,
	renderer func(tmpl string, vars map[string]string) (string, error),
	cfg *config.OverlayConfig,
	mode string,
	conflictPolicy ConflictPolicy,
) error {
	// Check if already installed.
	installed := isCloudflaredInstalled(ctx, exec)

	if !installed {
		if err := installCloudflared(ctx, exec); err != nil {
			return err
		}
	}

	// Handle existing config.
	if _, err := fs.Stat(cfConfigPath); err == nil {
		switch conflictPolicy {
		case ConflictSkip:
			return nil
		case ConflictBackup:
			backup := fmt.Sprintf("%s.%s.bak", cfConfigPath, time.Now().UTC().Format("20060102T150405Z"))
			data, _ := fs.ReadFile(cfConfigPath)
			if writeErr := fs.WriteFile(backup, data, 0o600); writeErr != nil {
				return fmt.Errorf("backup cloudflared config: %w", writeErr)
			}
		}
	}

	switch mode {
	case TunnelModeAccount:
		return setupAccountTunnel(ctx, exec, fs, renderer, cfg)
	case TunnelModeQuick:
		return setupQuickTunnel(ctx, exec, fs, cfg)
	default:
		return fmt.Errorf("unknown tunnel mode: %q", mode)
	}
}

func isCloudflaredInstalled(ctx context.Context, exec shell.Executor) bool {
	res, err := exec.Run(ctx, shell.ExecOpts{Cmd: []string{"cloudflared", "--version"}})
	return err == nil && res.ExitCode == 0
}

func installCloudflared(ctx context.Context, exec shell.Executor) error {
	// Download and install the latest cloudflared .deb.
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"bash", "-c",
			"curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o /tmp/cloudflared.deb"},
	}); err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"dpkg", "-i", "/tmp/cloudflared.deb"},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("dpkg install cloudflared: %w", err)
	}
	return nil
}

func setupAccountTunnel(
	ctx context.Context,
	exec shell.Executor,
	fs shell.FS,
	renderer func(string, map[string]string) (string, error),
	cfg *config.OverlayConfig,
) error {
	// Create named tunnel.
	res, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"cloudflared", "tunnel", "create", "openclaw-multi"},
	})
	if err != nil {
		return fmt.Errorf("cloudflared tunnel create: %w", err)
	}
	tunnelID := parseTunnelID(res.Stdout)
	if tunnelID == "" {
		return fmt.Errorf("could not parse tunnel ID from: %q", res.Stdout)
	}
	cfg.TunnelID = tunnelID

	// Route DNS.
	wildcardDomain := fmt.Sprintf("*.%s.%s", cfg.Subdomain, cfg.Domain)
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"cloudflared", "tunnel", "route", "dns", "openclaw-multi", wildcardDomain},
	}); err != nil {
		return fmt.Errorf("cloudflared route dns: %w", err)
	}

	// Render and write config.
	credFile := fmt.Sprintf("%s/%s.json", cfCredPath, tunnelID)
	content, err := renderer("templates/cloudflared-config.tmpl", map[string]string{
		"TUNNEL_ID":               tunnelID,
		"TUNNEL_CREDENTIALS_FILE": credFile,
	})
	if err != nil {
		return fmt.Errorf("render cloudflared config: %w", err)
	}
	if err := fs.WriteFile(cfConfigPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write cloudflared config: %w", err)
	}

	// Validate config.
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"cloudflared", "tunnel", "ingress", "validate", cfConfigPath},
	}); err != nil {
		return fmt.Errorf("cloudflared ingress validate: %w", err)
	}

	// Enable and start service.
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"systemctl", "enable", "--now", cfServiceName},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("systemctl enable cloudflared: %w", err)
	}
	return nil
}

func setupQuickTunnel(_ context.Context, exec shell.Executor, fs shell.FS, cfg *config.OverlayConfig) error {
	// Write a minimal quick-tunnel config.
	content := "# cloudflared quick tunnel — ephemeral URL, no account required\n" +
		"# Re-run cloudflared with: cloudflared tunnel --url http://localhost:18000\n"
	if err := fs.WriteFile(cfConfigPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write quick-tunnel config: %w", err)
	}
	cfg.TunnelMode = TunnelModeQuick
	_ = exec // quick tunnel does not need exec beyond writing config
	return nil
}

// parseTunnelID extracts the tunnel UUID from `cloudflared tunnel create` output.
// Output contains a line like: "Created tunnel openclaw-multi with id <uuid>"
func parseTunnelID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "with id") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}
