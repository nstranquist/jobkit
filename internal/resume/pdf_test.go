package resume

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPDFUsesConfiguredChrome(t *testing.T) {
	dir := t.TempDir()
	chrome := filepath.Join(dir, "fake-chrome")
	if err := os.WriteFile(chrome, []byte("test chrome marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JOBKIT_CHROME_BIN", chrome)
	previous := runChrome
	t.Cleanup(func() { runChrome = previous })
	runChrome = func(name string, args ...string) ([]byte, error) {
		if name != chrome {
			t.Fatalf("chrome = %q, want %q", name, chrome)
		}
		for _, arg := range args {
			if out, ok := strings.CutPrefix(arg, "--print-to-pdf="); ok {
				return nil, os.WriteFile(out, []byte("%PDF-1.4 fake"), 0o600)
			}
		}
		return nil, fmt.Errorf("missing pdf argument")
	}

	out := filepath.Join(dir, "resume.pdf")
	if err := RenderPDF("<!doctype html><title>Resume</title>", out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "%PDF-1.4 fake" {
		t.Fatalf("pdf = %q, want fake pdf content", string(raw))
	}
}
