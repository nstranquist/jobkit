package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPermissionsReportsThenExplicitlyFixesLegacyState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jobkit")
	t.Setenv("JOBKIT_HOME", root)
	if err := os.MkdirAll(filepath.Join(root, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "applications.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := CheckPermissions(false)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Fixed || len(report.Issues) < 2 {
		t.Fatalf("audit did not report legacy permissions: %#v", report)
	}
	report, err = CheckPermissions(true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || !report.Fixed {
		t.Fatalf("explicit repair failed: %#v", report)
	}
	for _, item := range []struct {
		path string
		want os.FileMode
	}{{root, 0o700}, {filepath.Join(root, "out"), 0o700}, {path, 0o600}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != item.want {
			t.Fatalf("%s mode = %o, want %o", item.path, info.Mode().Perm(), item.want)
		}
	}
}
