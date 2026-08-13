//go:build windows

package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPrivateACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, DirMode); err != nil {
		t.Fatal(err)
	}
	if err := Restrict(path, DirMode); err != nil {
		t.Fatal(err)
	}
	private, observed, err := Inspect(path, DirMode)
	if err != nil {
		t.Fatal(err)
	}
	if !private {
		t.Fatalf("directory protection is not private (mode %o)", observed)
	}
}
