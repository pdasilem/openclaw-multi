package deps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// TailscaleStatus describes the result of EnsureTailscale.
type TailscaleStatus struct {
	Installed bool
	Running   bool
	LoggedIn  bool
	IP        string
	Skipped   bool
}

type tailscaleStatusJSON struct {
	BackendState string   `json:"BackendState"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

// EnsureTailscale ensures Tailscale is installed and running.
// If interactive=false (always in tests/CI), skips the `tailscale up` step.
func EnsureTailscale(ctx context.Context, exec shell.Executor, interactive bool) (TailscaleStatus, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{Cmd: []string{"tailscale", "version"}})
	if err != nil || res.ExitCode != 0 {
		if err := installTailscale(ctx, exec); err != nil {
			return TailscaleStatus{}, err
		}
	}

	statusRes, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"tailscale", "status", "--json"},
		Sudo: true,
	})
	if err != nil {
		return TailscaleStatus{Installed: true}, fmt.Errorf("tailscale status: %w", err)
	}

	var status tailscaleStatusJSON
	if jsonErr := json.Unmarshal([]byte(statusRes.Stdout), &status); jsonErr != nil {
		return TailscaleStatus{Installed: true}, nil
	}

	loggedIn := status.BackendState == "Running"
	ip := ""
	if len(status.TailscaleIPs) > 0 {
		ip = status.TailscaleIPs[0]
	}

	if loggedIn {
		return TailscaleStatus{Installed: true, Running: true, LoggedIn: true, IP: ip, Skipped: true}, nil
	}
	if !interactive {
		return TailscaleStatus{Installed: true, Running: true, LoggedIn: false}, nil
	}

	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"tailscale", "up", "--ssh"},
		Sudo: true,
	}); err != nil {
		return TailscaleStatus{Installed: true}, fmt.Errorf("tailscale up: %w", err)
	}
	return TailscaleStatus{Installed: true, Running: true, LoggedIn: true}, nil
}

func installTailscale(ctx context.Context, exec shell.Executor) error {
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"bash", "-c", "curl -fsSL https://tailscale.com/install.sh | sudo sh"},
	}); err != nil {
		return fmt.Errorf("tailscale install: %w", err)
	}
	return nil
}
