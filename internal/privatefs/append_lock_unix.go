//go:build !windows

package privatefs

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockAppend(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}
