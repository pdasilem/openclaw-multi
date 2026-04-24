package shell

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FS is the interface for filesystem operations used by hardening and deps
// packages. RealFS uses the os package; MemFS is the in-memory test double.
type FS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldpath, newpath string) error
}

// RealFS delegates to the os package.
type RealFS struct{}

func (RealFS) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (RealFS) Stat(path string) (fs.FileInfo, error)        { return os.Stat(path) }
func (RealFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (RealFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }

func (RealFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// MemFS is an in-memory filesystem for tests.
type MemFS struct {
	Files map[string][]byte
}

// NewMemFS returns an initialised MemFS.
func NewMemFS() *MemFS { return &MemFS{Files: make(map[string][]byte)} }

func (m *MemFS) ReadFile(path string) ([]byte, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return data, nil
}

func (m *MemFS) WriteFile(path string, data []byte, _ fs.FileMode) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	m.Files[path] = cp
	return nil
}

func (m *MemFS) Stat(path string) (fs.FileInfo, error) {
	if _, ok := m.Files[path]; !ok {
		return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	return memFileInfo{name: filepath.Base(path)}, nil
}

func (m *MemFS) MkdirAll(_ string, _ fs.FileMode) error { return nil }

func (m *MemFS) Rename(oldpath, newpath string) error {
	data, ok := m.Files[oldpath]
	if !ok {
		return &os.PathError{Op: "rename", Path: oldpath, Err: os.ErrNotExist}
	}
	m.Files[newpath] = data
	delete(m.Files, oldpath)
	return nil
}

type memFileInfo struct{ name string }

func (f memFileInfo) Name() string     { return f.name }
func (memFileInfo) Size() int64        { return 0 }
func (memFileInfo) Mode() fs.FileMode  { return 0o600 }
func (memFileInfo) ModTime() time.Time { return time.Time{} }
func (memFileInfo) IsDir() bool        { return false }
func (memFileInfo) Sys() any           { return nil }
