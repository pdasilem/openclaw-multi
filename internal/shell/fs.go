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
	Remove(path string) error
}

// RealFS delegates to the os package.
type RealFS struct{}

func (RealFS) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (RealFS) Stat(path string) (fs.FileInfo, error)        { return os.Stat(path) }
func (RealFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (RealFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (RealFS) Remove(path string) error                     { return os.Remove(path) }

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
	Modes map[string]fs.FileMode
}

// NewMemFS returns an initialised MemFS.
func NewMemFS() *MemFS {
	return &MemFS{Files: make(map[string][]byte), Modes: make(map[string]fs.FileMode)}
}

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
		if mode, mok := m.Modes[path]; mok {
			return memFileInfo{name: filepath.Base(path), mode: mode}, nil
		}
		return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	mode := fs.FileMode(0o600)
	if configured, ok := m.Modes[path]; ok {
		mode = configured
	}
	return memFileInfo{name: filepath.Base(path), mode: mode}, nil
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

func (m *MemFS) Remove(path string) error {
	if _, ok := m.Files[path]; !ok {
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	delete(m.Files, path)
	return nil
}

type memFileInfo struct {
	name string
	mode fs.FileMode
}

func (f memFileInfo) Name() string      { return f.name }
func (memFileInfo) Size() int64         { return 0 }
func (f memFileInfo) Mode() fs.FileMode { return f.mode }
func (memFileInfo) ModTime() time.Time  { return time.Time{} }
func (f memFileInfo) IsDir() bool       { return f.mode.IsDir() }
func (memFileInfo) Sys() any            { return nil }
