// Package privatefs provides fail-closed filesystem primitives for JobKit's
// personal profile, application, inbox, and generated-artifact data.
package privatefs

import (
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

// OpenAppend opens an append-only private ledger. Existing file permissions
// are intentionally left unchanged so `jobkit doctor permissions` can report
// and explicitly repair legacy state.
func OpenAppend(path string) (*os.File, error) {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, FileMode)
}
