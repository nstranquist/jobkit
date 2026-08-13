package privatefs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
		private, observed, err := Inspect(item.path, item.want)
		if err != nil {
			t.Fatal(err)
		}
		if !private {
			t.Fatalf("%s protection is not private (mode %o, want %o)", item.path, observed, item.want)
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

func TestOpenAppendSerializesConcurrentRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	const writers = 24
	start := make(chan struct{})
	errs := make(chan error, writers)
	var group sync.WaitGroup
	for i := 0; i < writers; i++ {
		group.Add(1)
		go func(id int) {
			defer group.Done()
			<-start
			file, err := OpenAppend(path)
			if err != nil {
				errs <- err
				return
			}
			_, writeErr := fmt.Fprintf(file, "{\"id\":%d}\n", id)
			closeErr := file.Close()
			if writeErr != nil {
				errs <- writeErr
			} else if closeErr != nil {
				errs <- closeErr
			}
		}(i)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		seen[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != writers {
		t.Fatalf("records = %d, want %d", len(seen), writers)
	}
}
