package resume

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nstranquist/jobkit/internal/privatefs"
)

func RenderPDF(htmlContent, outPath string) error {
	if strings.TrimSpace(outPath) == "" {
		return fmt.Errorf("pdf output path is required")
	}
	chrome, err := findChrome()
	if err != nil {
		return err
	}
	if err := privatefs.EnsureDir(filepath.Dir(outPath)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "jobkit-resume-*.html")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	n, err := tmp.WriteString(htmlContent)
	if err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("write temp html: %w; close: %v", err, closeErr)
		}
		return err
	}
	if n != len(htmlContent) {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("write temp html: %w; close: %v", io.ErrShortWrite, closeErr)
		}
		return io.ErrShortWrite
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--print-to-pdf=" + outPath,
		"file://" + tmpPath,
	}
	cmd := exec.Command(chrome, args...)
	if raw, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(raw))
		if msg != "" {
			return fmt.Errorf("chrome print failed: %w: %s", err, msg)
		}
		return fmt.Errorf("chrome print failed: %w", err)
	}
	if info, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("pdf was not written: %w", err)
	} else if info.IsDir() || info.Size() == 0 {
		return fmt.Errorf("pdf was not written or is empty: %s", outPath)
	}
	return os.Chmod(outPath, privatefs.FileMode)
}

func findChrome() (string, error) {
	if v := strings.TrimSpace(os.Getenv("JOBKIT_CHROME_BIN")); v != "" {
		if info, err := os.Stat(v); err == nil && !info.IsDir() {
			return v, nil
		}
		return "", fmt.Errorf("JOBKIT_CHROME_BIN does not point to an executable file: %s", v)
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("headless Chrome/Chromium not found; set JOBKIT_CHROME_BIN")
}
