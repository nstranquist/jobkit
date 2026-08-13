//go:build !windows

package privatefs

import "os"

func restrictPath(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func inspectPath(path string, want os.FileMode) (bool, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	observed := info.Mode().Perm()
	return observed == want, observed, nil
}
