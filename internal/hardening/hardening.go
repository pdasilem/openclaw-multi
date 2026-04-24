// Package hardening applies host security hardening: sysctl, hidepid, profile.d.
// All operations are idempotent and go through injectable Executor + FS interfaces.
package hardening

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/shell"
)

const (
	sysctlConf   = "/etc/sysctl.d/openclaw-overlay.conf"
	fstabPath    = "/etc/fstab"
	hidepidLine  = "proc /proc proc defaults,hidepid=2,gid=adm 0 0"
	profilePath  = "/etc/profile.d/openclaw.sh"
	compileCache = "/var/cache/openclaw-compile"
)

var sysctlContent = []byte(`# openclaw-multi host hardening
kernel.yama.ptrace_scope = 2
kernel.dmesg_restrict = 1
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
`)

var profileContent = []byte(`#!/bin/bash
# openclaw-multi: per-user environment hardening
umask 0077
export NODE_COMPILE_CACHE=/var/cache/openclaw-compile
`)

// Hardener applies host hardening using injectable dependencies.
type Hardener struct {
	Exec   shell.Executor
	FS     shell.FS
	Logger shell.Logger
}

// ApplySysctl writes /etc/sysctl.d/openclaw-overlay.conf and runs sysctl --system.
// Idempotent: if the file already contains the expected content, no write occurs.
func (h *Hardener) ApplySysctl(ctx context.Context) error {
	existing, err := h.FS.ReadFile(sysctlConf)
	if err == nil && bytes.Equal(existing, sysctlContent) {
		h.emit(sysctlConf, "sysctl: already configured, skip")
		return nil
	}
	if err := h.FS.WriteFile(sysctlConf, sysctlContent, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sysctlConf, err)
	}
	if _, err := h.Exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"sysctl", "--system"},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("sysctl --system: %w", err)
	}
	h.emit(sysctlConf, "sysctl applied")
	return nil
}

// ApplyHidepid adds the hidepid=2 mount option to /etc/fstab if not present,
// then remounts /proc.
func (h *Hardener) ApplyHidepid(ctx context.Context) error {
	data, _ := h.FS.ReadFile(fstabPath)
	if bytes.Contains(data, []byte("hidepid=2")) {
		h.emit(fstabPath, "hidepid: already configured, skip")
		return nil
	}
	newData := make([]byte, 0, len(data)+len(hidepidLine)+2)
	newData = append(newData, data...)
	newData = append(newData, '\n')
	newData = append(newData, []byte(hidepidLine+"\n")...)
	if err := h.FS.WriteFile(fstabPath, newData, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fstabPath, err)
	}
	if _, err := h.Exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"mount", "-o", "remount", "/proc"},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("mount remount /proc: %w", err)
	}
	h.emit(fstabPath, "hidepid applied")
	return nil
}

// ApplyProfile writes /etc/profile.d/openclaw.sh and creates the compile cache dir.
// Idempotent: if profile already exists with correct content, no write occurs.
func (h *Hardener) ApplyProfile(ctx context.Context) error {
	existing, err := h.FS.ReadFile(profilePath)
	if err == nil && bytes.Equal(existing, profileContent) {
		h.emit(profilePath, "profile: already configured, skip")
	} else {
		if err := h.FS.WriteFile(profilePath, profileContent, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", profilePath, err)
		}
	}
	if _, err := h.Exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"mkdir", "-p", compileCache},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("mkdir %s: %w", compileCache, err)
	}
	if _, err := h.Exec.Run(ctx, shell.ExecOpts{
		Cmd:  []string{"chmod", "1777", compileCache},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("chmod %s: %w", compileCache, err)
	}
	h.emit(profilePath, "profile applied")
	return nil
}

func (h *Hardener) emit(target, detail string) {
	if h.Logger == nil {
		return
	}
	_ = h.Logger.Emit(audit.Event{
		Actor:   "system",
		Action:  audit.ActionShellExec,
		Target:  target,
		Result:  audit.ResultOk,
		Details: map[string]any{"detail": detail},
	})
}
