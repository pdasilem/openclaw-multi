package systemprep

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
)

const (
	overlayConfigPath     = "/etc/openclaw-multi/config.yml"
	cloudflaredConfigPath = "/etc/cloudflared/config.yml"
)

// Run prepares root-owned directories so the admin user can run the TUI without sudo.
func Run(ctx context.Context, exec shell.Executor) ([]string, error) {
	if os.Getuid() != 0 {
		return nil, fmt.Errorf("system-prepare must be run with sudo")
	}
	adminUser := os.Getenv("SUDO_USER")
	if adminUser == "" || adminUser == "root" {
		return nil, fmt.Errorf("system-prepare must be run as sudo from the admin user, not from a root shell")
	}
	u, err := user.Lookup(adminUser)
	if err != nil {
		return nil, fmt.Errorf("lookup admin user %q: %w", adminUser, err)
	}
	actions := make([]string, 0, 4)
	for _, dir := range []string{"/var/lib/openclaw-multi", "/var/log/openclaw-multi"} {
		if _, err := exec.Run(ctx, shell.ExecOpts{
			Cmd: []string{"install", "-d", "-m", "0700", "-o", adminUser, "-g", u.Gid, dir},
		}); err != nil {
			return actions, fmt.Errorf("prepare %s: %w", dir, err)
		}
		actions = append(actions, fmt.Sprintf("ensured %s owner=%s group=%s mode=0700", dir, adminUser, u.Gid))
		if _, err := exec.Run(ctx, shell.ExecOpts{
			Cmd: []string{"chown", "-R", adminUser + ":" + u.Gid, dir},
		}); err != nil {
			return actions, fmt.Errorf("chown %s: %w", dir, err)
		}
		actions = append(actions, fmt.Sprintf("repaired existing ownership under %s owner=%s group=%s", dir, adminUser, u.Gid))
	}
	for _, dir := range []string{"/etc/openclaw-multi", "/etc/cloudflared"} {
		if _, err := exec.Run(ctx, shell.ExecOpts{
			Cmd: []string{"install", "-d", "-m", "0755", dir},
		}); err != nil {
			return actions, fmt.Errorf("prepare %s: %w", dir, err)
		}
		actions = append(actions, fmt.Sprintf("ensured %s owner=root group=root mode=0755", dir))
	}
	configActions, err := reconcileConfigs(bufio.NewReader(os.Stdin))
	actions = append(actions, configActions...)
	if err != nil {
		return actions, err
	}
	return actions, nil
}

func reconcileConfigs(in *bufio.Reader) ([]string, error) {
	var actions []string
	action, edit, err := reconcileOverlayConfig(in)
	if action != "" {
		actions = append(actions, action)
	}
	if err != nil {
		return actions, err
	}
	if edit {
		if err := editFile(overlayConfigPath); err != nil {
			return actions, err
		}
		actions = append(actions, fmt.Sprintf("edited %s", overlayConfigPath))
	}
	action, needsEdit, err := reconcileCloudflaredConfig(in)
	if needsEdit && err == nil {
		if err := editFile(overlayConfigPath); err != nil {
			return actions, err
		}
		actions = append(actions, fmt.Sprintf("edited %s", overlayConfigPath))
		action, _, err = reconcileCloudflaredConfig(in)
	}
	if action != "" {
		actions = append(actions, action)
	}
	return actions, err
}

func reconcileOverlayConfig(in *bufio.Reader) (string, bool, error) {
	data, err := yaml.Marshal(config.Defaults())
	if err != nil {
		return "", false, fmt.Errorf("marshal default overlay config: %w", err)
	}
	if _, err := os.Stat(overlayConfigPath); err != nil {
		if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("stat %s: %w", overlayConfigPath, err)
		}
		if err := writeRootFile(overlayConfigPath, data); err != nil {
			return "", false, err
		}
		return fmt.Sprintf("created %s mode=0600", overlayConfigPath), true, nil
	}
	replace, err := askReplace(in, overlayConfigPath)
	if err != nil || !replace {
		return fmt.Sprintf("kept existing %s", overlayConfigPath), false, err
	}
	if err := backupFile(overlayConfigPath); err != nil {
		return "", false, err
	}
	if err := writeRootFile(overlayConfigPath, data); err != nil {
		return "", false, err
	}
	return fmt.Sprintf("replaced %s with default config after backup", overlayConfigPath), true, nil
}

func reconcileCloudflaredConfig(in *bufio.Reader) (string, bool, error) {
	if _, err := os.Stat(cloudflaredConfigPath); err != nil && !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat %s: %w", cloudflaredConfigPath, err)
	} else if err == nil {
		replace, err := askReplace(in, cloudflaredConfigPath)
		if err != nil || !replace {
			return fmt.Sprintf("kept existing %s", cloudflaredConfigPath), false, err
		}
	}
	cfg, err := config.Load(overlayConfigPath)
	if err != nil {
		return fmt.Sprintf("skipped %s: %s is not ready", cloudflaredConfigPath, overlayConfigPath), false, nil
	}
	if strings.TrimSpace(cfg.TunnelID) == "" || strings.TrimSpace(cfg.CloudflaredCredentialsFile) == "" {
		return fmt.Sprintf("waiting for %s: fill tunnel_id and cloudflared_credentials_file", cloudflaredConfigPath), true, nil
	}
	content := []byte(fmt.Sprintf("tunnel: %s\ncredentials-file: %s\n\ningress:\n  - service: http_status:404\n",
		cfg.TunnelID, cfg.CloudflaredCredentialsFile))
	if _, err := os.Stat(cloudflaredConfigPath); err == nil {
		if err := backupFile(cloudflaredConfigPath); err != nil {
			return "", false, err
		}
	}
	if err := writeRootFile(cloudflaredConfigPath, content); err != nil {
		return "", false, err
	}
	return fmt.Sprintf("created %s mode=0600", cloudflaredConfigPath), false, nil
}

func askReplace(in *bufio.Reader, path string) (bool, error) {
	for {
		fmt.Fprintf(os.Stderr, "%s exists. Keep existing? [K]eep/[R]eplace/[A]bort: ", path)
		answer, err := in.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read answer for %s: %w", path, err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "k", "keep":
			return false, nil
		case "r", "replace":
			return true, nil
		case "a", "abort":
			return false, fmt.Errorf("aborted while reconciling %s", path)
		}
	}
}

func backupFile(path string) error {
	backup := fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102T150405Z"))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s for backup: %w", path, err)
	}
	if err := writeRootFile(backup, data); err != nil {
		return fmt.Errorf("write backup %s: %w", backup, err)
	}
	return nil
}

func writeRootFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}
	return nil
}

func editFile(path string) error {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "nano"
	}
	cmd := exec.Command(editor, path) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("edit %s with %s: %w", path, editor, err)
	}
	return nil
}
