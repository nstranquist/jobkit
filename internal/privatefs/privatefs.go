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
	missing := make([]string, 0, 2)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		_, err := os.Stat(current)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect private directory %s: %w", current, err)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if err := os.MkdirAll(path, DirMode); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := Restrict(missing[index], DirMode); err != nil {
			return fmt.Errorf("protect private directory %s: %w", missing[index], err)
		}
	}
	return nil
}

// Restrict applies JobKit's private file or directory protection. Unix uses
// mode bits. Windows uses a protected access-control list for the current user.
func Restrict(path string, mode os.FileMode) error {
	if mode != DirMode && mode != FileMode {
		return fmt.Errorf("unsupported private mode %o", mode)
	}
	return restrictPath(path, mode)
}

// Inspect reports whether path has JobKit's private protection. The observed
// mode is included for diagnostics; Windows authorization is decided by the
// access-control list, not by emulated POSIX mode bits.
func Inspect(path string, want os.FileMode) (private bool, observed os.FileMode, err error) {
	if want != DirMode && want != FileMode {
		return false, 0, fmt.Errorf("unsupported private mode %o", want)
	}
	return inspectPath(path, want)
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
	if err := Restrict(tempPath, FileMode); err != nil {
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
	return Restrict(path, FileMode)
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
	file, err := openPrivateFile(path, os.O_APPEND|os.O_WRONLY)
	if err != nil {
		_ = pathLock.Close()
		return nil, err
	}
	return &AppendFile{file: file, lock: pathLock}, nil
}

// WithPathLock serializes a complete read-modify-replace transaction with
// OpenAppend. The callback must not call OpenAppend for the same path.
func WithPathLock(path string, run func() error) (err error) {
	pathLock, err := openPathLock(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, pathLock.Close()) }()
	return run()
}

func openPathLock(path string) (*os.File, error) {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	file, err := openPrivateFile(lockPath, os.O_RDWR)
	if err != nil {
		return nil, err
	}
	if err := lockAppend(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock private path %s: %w", path, err)
	}
	return file, nil
}

// openPrivateFile creates a new private file without changing the protection
// of an existing legacy file. The permission doctor owns explicit repair.
func openPrivateFile(path string, flags int) (*os.File, error) {
	file, err := os.OpenFile(path, flags|os.O_CREATE|os.O_EXCL, FileMode)
	if err == nil {
		if protectErr := Restrict(path, FileMode); protectErr != nil {
			_ = file.Close()
			return nil, protectErr
		}
		return file, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return os.OpenFile(path, flags, FileMode)
}
