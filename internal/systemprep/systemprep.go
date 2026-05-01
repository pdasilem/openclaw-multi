package systemprep

import (
	"context"
	"fmt"
	"os"
	"os/user"

	"github.com/pdasilem/openclaw-multi/internal/shell"
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
	return actions, nil
}
