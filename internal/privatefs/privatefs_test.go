package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivatePrimitivesUseRestrictiveModes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(root, "nested", "state.json")
	if err := WriteFile(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		path string
		want os.FileMode
	}{{root, DirMode}, {filepath.Join(root, "nested"), DirMode}, {path, FileMode}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != item.want {
			t.Fatalf("%s mode = %o, want %o", item.path, got, item.want)
		}
	}
	if err := WriteFile(path, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "two\n" {
		t.Fatalf("atomic replacement = %q, %v", body, err)
	}
}
