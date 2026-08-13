// Package privatefs provides fail-closed filesystem primitives for JobKit's
// personal profile, application, inbox, and generated-artifact data.
package privatefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirMode  = 0o700
	FileMode = 0o600
)

func EnsureDir(path string) error {
	if err := os.MkdirAll(path, DirMode); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	return nil
}

// WriteFile atomically replaces path with a private regular file.
func WriteFile(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".jobkit-write-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(FileMode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return os.Chmod(path, FileMode)
}

// AppendFile holds the stable path lock for an append-only ledger until Close.
// The separate lock file keeps atomic replacement and append operations on the
// same lock even when the ledger inode changes during a rename.
type AppendFile struct {
	file *os.File
	lock *os.File
}

func (f *AppendFile) Write(payload []byte) (int, error) {
	return f.file.Write(payload)
}

func (f *AppendFile) Sync() error {
	return f.file.Sync()
}

func (f *AppendFile) Close() error {
	return errors.Join(f.file.Close(), f.lock.Close())
}

// OpenAppend opens an append-only private ledger and holds its stable path
// lock. Existing ledger permissions are intentionally left unchanged so
// `jobkit doctor permissions` can report and explicitly repair legacy state.
func OpenAppend(path string) (*AppendFile, error) {
	pathLock, err := openPathLock(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, FileMode)
	if err != nil {
		_ = pathLock.Close()
		return nil, err
	}
	return &AppendFile{file: file, lock: pathLock}, nil
}

// WithPathLock serializes a complete read-modify-replace transaction with
// OpenAppend. The callback must not call OpenAppend for the same path.
func WithPathLock(path string, run func() error) error {
	pathLock, err := openPathLock(path)
	if err != nil {
		return err
	}
	defer pathLock.Close()
	return run()
}

func openPathLock(path string) (*os.File, error) {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, FileMode)
	if err != nil {
		return nil, err
	}
	if err := lockAppend(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock private path %s: %w", path, err)
	}
	return file, nil
}
