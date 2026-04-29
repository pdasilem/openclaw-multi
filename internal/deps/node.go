// Package deps provides idempotent ensure-functions for system dependencies.
// All functions take a shell.Executor and never touch the real system in tests.
package deps

import (
	"context"
	"strconv"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// NodeStatus describes the result of EnsureNode.
type NodeStatus struct {
	Installed bool
	Version   string
	Skipped   bool // true when no system-space Node action is required
}

// EnsureNode checks for a sufficient system Node.js without installing it.
// OpenClaw tenant runtimes are installed per managed user through nvm, so fresh
// install must not add Node.js to system space.
func EnsureNode(ctx context.Context, exec shell.Executor, minVersion string) (NodeStatus, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{Cmd: []string{"node", "--version"}})
	if err == nil {
		version := strings.TrimSpace(res.Stdout)
		if semverGTE(version, minVersion) {
			return NodeStatus{Installed: true, Version: version, Skipped: true}, nil
		}
		return NodeStatus{Installed: true, Version: version, Skipped: true}, nil
	}

	return NodeStatus{Installed: false, Skipped: true}, nil
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
