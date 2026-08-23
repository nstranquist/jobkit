package blacklist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchIgnoresCaseAndPunctuation(t *testing.T) {
	list := List{Companies: []string{"Acme Corp."}}
	if hit := list.Match("acme corp"); hit == nil || hit.Matched != "Acme Corp." {
		t.Fatalf("hit=%#v", hit)
	}
	if hit := list.Match("Initech"); hit != nil {
		t.Fatalf("unexpected hit %#v", hit)
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	list, err := Load(filepath.Join(t.TempDir(), "blacklist.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if hit := list.Match("Acme"); hit != nil {
		t.Fatalf("empty list hit=%#v", hit)
	}
}

func TestLoadUserFileNeverRequiresJobkitWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blacklist.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 1\ncompanies:\n  - Noise\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if hit := list.Match("noise"); hit == nil {
		t.Fatal("expected hit")
	}
}
