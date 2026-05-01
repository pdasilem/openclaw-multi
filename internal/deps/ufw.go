package deps

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// UFWStatus describes the result of EnsureUFW.
type UFWStatus struct {
	Active        bool
	DenyIncoming  bool
	AllowOutgoing bool
	AddedPorts    []int
	AllowedPorts  []int // ports already open before our run
}

// EnsureUFW configures UFW idempotently.
// If UFW is inactive, it sets up from scratch.
// If already active, only missing rules are added.
func EnsureUFW(ctx context.Context, exec shell.Executor, extraPorts []int) (UFWStatus, error) {
	status, err := getUFWStatus(ctx, exec)
	if err != nil {
		return UFWStatus{}, fmt.Errorf("ufw status: %w", err)
	}

	var added []int

	if !status.Active {
		cmds := [][]string{
			{"ufw", "default", "deny", "incoming"},
			{"ufw", "default", "allow", "outgoing"},
		}
		for _, port := range extraPorts {
			cmds = append(cmds, []string{"ufw", "allow", fmt.Sprintf("%d/tcp", port)})
		}
		cmds = append(cmds, []string{"ufw", "--force", "enable"})
		for _, cmd := range cmds {
			if _, err := exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true}); err != nil {
				return UFWStatus{}, fmt.Errorf("ufw %v: %w", cmd, err)
			}
		}
		added = extraPorts
	} else {
		if !status.DenyIncoming {
			for _, cmd := range [][]string{
				{"ufw", "default", "deny", "incoming"},
				{"ufw", "reload"},
			} {
				if _, err := exec.Run(ctx, shell.ExecOpts{Cmd: cmd, Sudo: true}); err != nil {
					return status, fmt.Errorf("ufw %v: %w", cmd, err)
				}
			}
			status.DenyIncoming = true
		}
		for _, port := range extraPorts {
			if !portAllowed(status.AllowedPorts, port) {
				if _, err := exec.Run(ctx, shell.ExecOpts{
					Cmd:  []string{"ufw", "allow", fmt.Sprintf("%d/tcp", port)},
					Sudo: true,
				}); err != nil {
					return status, fmt.Errorf("ufw allow %d: %w", port, err)
				}
				added = append(added, port)
			}
		}
	}

	status.Active = true
	status.DenyIncoming = true
	status.AllowOutgoing = true
	status.AddedPorts = added
	return status, nil
}

func getUFWStatus(ctx context.Context, exec shell.Executor) (UFWStatus, error) {
	res, err := exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"ufw", "status", "verbose"},
		Sudo: true,
	})
	if err != nil {
		return UFWStatus{}, err
	}
	return parseUFWStatus(res.Stdout), nil
}

func parseUFWStatus(output string) UFWStatus {
	var s UFWStatus
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Status: active"):
			s.Active = true
		case strings.Contains(line, "Default: deny (incoming)"):
			s.DenyIncoming = true
		case strings.Contains(line, "Default: allow (outgoing)"):
			s.AllowOutgoing = true
		}
		if strings.Contains(line, "ALLOW") && strings.Contains(line, "tcp") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				portStr := strings.TrimSuffix(parts[0], "/tcp")
				if port, err := strconv.Atoi(portStr); err == nil {
					s.AllowedPorts = append(s.AllowedPorts, port)
				}
			}
		}
	}
	return s
}

func portAllowed(ports []int, port int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}
