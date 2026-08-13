package main

import "testing"

func TestApprovedDependencySetIsExplicit(t *testing.T) {
	if len(approved) != 3 {
		t.Fatalf("approved dependency count = %d, want 3", len(approved))
	}
	for path, license := range approved {
		if license.Version == "" || license.Expression == "" || len(license.SHA256) != 64 {
			t.Fatalf("incomplete approval for %s: %#v", path, license)
		}
	}
}

func TestVerifyRejectsUnreviewedDependency(t *testing.T) {
	err := verify([]dependency{{Path: "example.com/unreviewed", Version: "v1.0.0", Dir: t.TempDir()}})
	if err == nil {
		t.Fatal("verify accepted an unreviewed dependency")
	}
}
