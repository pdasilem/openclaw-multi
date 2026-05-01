package deps

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
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

var tunnelIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

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
	cli, err := cloudflaredAccountCLI()
	if err != nil {
		return err
	}

	tunnelName := strings.TrimSpace(cfg.TunnelName)
	tunnelID := strings.TrimSpace(cfg.TunnelID)
	tunnelRef := tunnelID
	if tunnelID == "" {
		if tunnelName == "" {
			return fmt.Errorf("cloudflared tunnel_name is required when tunnel_id is empty")
		}
		tunnelID, err = findExistingTunnel(ctx, exec, cli, tunnelName)
		if err != nil {
			return err
		}
	}
	if tunnelID == "" {
		res, err := exec.Run(ctx, shell.ExecOpts{
			Cmd: cli.command("tunnel", "create", tunnelName),
		})
		if err != nil {
			return fmt.Errorf("cloudflared tunnel create: %w", err)
		}
		tunnelID = parseTunnelID(res.Stdout + "\n" + res.Stderr)
		tunnelRef = tunnelName
	} else if tunnelName != "" {
		tunnelRef = tunnelName
	}
	if tunnelID == "" {
		return fmt.Errorf("could not determine tunnel ID for %q", tunnelName)
	}
	cfg.TunnelID = tunnelID

	// Route DNS.
	wildcardDomain := fmt.Sprintf("*.%s.%s", cfg.Subdomain, cfg.Domain)
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: cli.command("tunnel", "route", "dns", "--overwrite-dns", tunnelRef, wildcardDomain),
	}); err != nil {
		return fmt.Errorf("cloudflared route dns: %w", err)
	}

	// Render and write config.
	credFile := fmt.Sprintf("%s/%s.json", cfCredPath, tunnelID)
	if err := installTunnelCredentials(fs, cli.credentialsPath(tunnelID), credFile); err != nil {
		return err
	}
	cfg.CloudflaredCredentialsFile = credFile
	content, err := renderer("cloudflared-config.tmpl", map[string]string{
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
		Cmd:  []string{"cloudflared", "tunnel", "ingress", "validate", cfConfigPath},
		Sudo: true,
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

type accountCLI struct {
	user       string
	home       string
	originCert string
}

func cloudflaredAccountCLI() (accountCLI, error) {
	if os.Getuid() != 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return accountCLI{}, fmt.Errorf("detect cloudflared user home: %w", err)
		}
		return accountCLI{home: home, originCert: filepath.Join(home, ".cloudflared", "cert.pem")}, nil
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" || sudoUser == "root" {
		return accountCLI{}, fmt.Errorf("cloudflared account setup must run as sudo from the admin user, not from a root shell")
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return accountCLI{}, fmt.Errorf("lookup SUDO_USER %q: %w", sudoUser, err)
	}
	return accountCLI{
		user:       sudoUser,
		home:       u.HomeDir,
		originCert: filepath.Join(u.HomeDir, ".cloudflared", "cert.pem"),
	}, nil
}

func (c accountCLI) command(args ...string) []string {
	cmd := []string{"cloudflared", "tunnel", "--origincert", c.originCert}
	if len(args) > 0 && args[0] == "tunnel" {
		args = args[1:]
	}
	cmd = append(cmd, args...)
	if c.user == "" {
		return cmd
	}
	return append([]string{"sudo", "-u", c.user, "-H"}, cmd...)
}

func (c accountCLI) credentialsPath(tunnelID string) string {
	return filepath.Join(c.home, ".cloudflared", tunnelID+".json")
}

func findExistingTunnel(ctx context.Context, exec shell.Executor, cli accountCLI, tunnelName string) (string, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: cli.command("tunnel", "list", "--name", tunnelName, "--output", "json"),
	})
	if err != nil {
		return "", fmt.Errorf("cloudflared tunnel list: %w", err)
	}
	return parseTunnelID(res.Stdout + "\n" + res.Stderr), nil
}

func installTunnelCredentials(fs shell.FS, sourcePath, targetPath string) error {
	data, err := fs.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read cloudflared tunnel credentials %q: %w", sourcePath, err)
	}
	if err := fs.WriteFile(targetPath, data, 0o600); err != nil {
		return fmt.Errorf("write cloudflared tunnel credentials %q: %w", targetPath, err)
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
	if id := tunnelIDPattern.FindString(output); id != "" {
		return id
	}
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
