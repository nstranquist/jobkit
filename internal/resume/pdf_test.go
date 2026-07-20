package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPDFUsesConfiguredChrome(t *testing.T) {
	dir := t.TempDir()
	chrome := filepath.Join(dir, "fake-chrome")
	script := `#!/bin/sh
out=""
for arg in "$@"; do
  case "$arg" in
    --print-to-pdf=*) out="${arg#--print-to-pdf=}" ;;
  esac
done
if [ -z "$out" ]; then
  echo "missing pdf arg" >&2
  exit 2
fi
printf '%s' '%PDF-1.4 fake' > "$out"
`
	if err := os.WriteFile(chrome, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JOBKIT_CHROME_BIN", chrome)

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
