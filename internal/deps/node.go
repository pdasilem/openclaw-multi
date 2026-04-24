// Package deps provides idempotent ensure-functions for system dependencies.
// All functions take a shell.Executor and never touch the real system in tests.
package deps

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// NodeStatus describes the result of EnsureNode.
type NodeStatus struct {
	Installed bool
	Version   string
	Skipped   bool // true if already at or above minVersion
}

// EnsureNode ensures Node.js >= minVersion is installed.
// If already present at a sufficient version, returns Skipped=true immediately.
func EnsureNode(ctx context.Context, exec shell.Executor, minVersion string) (NodeStatus, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{Cmd: []string{"node", "--version"}})
	if err == nil {
		version := strings.TrimSpace(res.Stdout)
		if semverGTE(version, minVersion) {
			return NodeStatus{Installed: true, Version: version, Skipped: true}, nil
		}
	}

	// Install via NodeSource.
	distro, _ := detectDistro(ctx, exec)
	if distro != "debian" && distro != "ubuntu" {
		return NodeStatus{}, fmt.Errorf("unsupported distro for Node.js install: %q", distro)
	}

	major := semverMajor(minVersion)
	setupURL := fmt.Sprintf("https://deb.nodesource.com/setup_%d.x", major)

	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"bash", "-c", fmt.Sprintf("curl -fsSL %s | sudo -E bash -", setupURL)},
		Sudo: false,
	}); err != nil {
		return NodeStatus{}, fmt.Errorf("nodesource setup: %w", err)
	}
	if _, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"apt-get", "install", "-y", "nodejs"},
		Sudo: true,
	}); err != nil {
		return NodeStatus{}, fmt.Errorf("apt-get install nodejs: %w", err)
	}

	verRes, _ := exec.Run(ctx, shell.ExecOpts{Cmd: []string{"node", "--version"}})
	return NodeStatus{Installed: true, Version: strings.TrimSpace(verRes.Stdout)}, nil
}

func detectDistro(ctx context.Context, exec shell.Executor) (string, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{
		Cmd: []string{"bash", "-c", "source /etc/os-release && echo $ID"},
	})
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(res.Stdout)), nil
}

// semverGTE returns true if version >= min (both in vX.Y.Z format).
func semverGTE(version, min string) bool {
	vp := parseSemver(strings.TrimPrefix(version, "v"))
	mp := parseSemver(strings.TrimPrefix(min, "v"))
	for i := range 3 {
		if vp[i] > mp[i] {
			return true
		}
		if vp[i] < mp[i] {
			return false
		}
	}
	return true
}

func parseSemver(s string) [3]int {
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func semverMajor(s string) int {
	return parseSemver(strings.TrimPrefix(s, "v"))[0]
}
