package shell

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PrivilegedFS writes system paths via sudo while keeping normal paths direct.
type PrivilegedFS struct {
	Base FS
	Exec Executor
}

func (p PrivilegedFS) base() FS {
	if p.Base != nil {
		return p.Base
	}
	return RealFS{}
}

func (p PrivilegedFS) ReadFile(path string) ([]byte, error)  { return p.base().ReadFile(path) }
func (p PrivilegedFS) Stat(path string) (fs.FileInfo, error) { return p.base().Stat(path) }

func (p PrivilegedFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if !needsSudoPath(path) {
		return p.base().WriteFile(path, data, perm)
	}
	if p.Exec == nil {
		return fmt.Errorf("write %s: privileged executor is required", path)
	}
	tmp, err := os.CreateTemp("", "openclaw-multi-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	mode := fmt.Sprintf("%04o", uint32(perm.Perm()))
	if _, err := p.Exec.Run(context.Background(), ExecOpts{
		Cmd:  []string{"install", "-D", "-m", mode, tmpPath, path},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("sudo install %s: %w", path, err)
	}
	return nil
}

func (p PrivilegedFS) MkdirAll(path string, perm fs.FileMode) error {
	if !needsSudoPath(path) {
		return p.base().MkdirAll(path, perm)
	}
	if p.Exec == nil {
		return fmt.Errorf("mkdir %s: privileged executor is required", path)
	}
	mode := fmt.Sprintf("%04o", uint32(perm.Perm()))
	if _, err := p.Exec.Run(context.Background(), ExecOpts{
		Cmd:  []string{"install", "-d", "-m", mode, path},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("sudo mkdir %s: %w", path, err)
	}
	return nil
}

func (p PrivilegedFS) Rename(oldpath, newpath string) error {
	if !needsSudoPath(oldpath) && !needsSudoPath(newpath) {
		return p.base().Rename(oldpath, newpath)
	}
	if p.Exec == nil {
		return fmt.Errorf("rename %s to %s: privileged executor is required", oldpath, newpath)
	}
	if _, err := p.Exec.Run(context.Background(), ExecOpts{
		Cmd:  []string{"mv", oldpath, newpath},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("sudo mv %s %s: %w", oldpath, newpath, err)
	}
	return nil
}

func (p PrivilegedFS) Remove(path string) error {
	if !needsSudoPath(path) {
		return p.base().Remove(path)
	}
	if p.Exec == nil {
		return fmt.Errorf("remove %s: privileged executor is required", path)
	}
	if _, err := p.Exec.Run(context.Background(), ExecOpts{
		Cmd:  []string{"rm", "-f", path},
		Sudo: true,
	}); err != nil {
		return fmt.Errorf("sudo rm %s: %w", path, err)
	}
	return nil
}

func needsSudoPath(path string) bool {
	clean := filepath.Clean(path)
	for _, prefix := range []string{"/etc", "/run", "/usr/local", "/opt", "/var/cache"} {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	return false
}
