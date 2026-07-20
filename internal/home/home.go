// Package home resolves jobkit's state directory (~/.jobkit by default,
// overridable via JOBKIT_HOME for tests and alternate vaults).
package home

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the jobkit state directory, creating it if needed.
func Dir() (string, error) {
	dir := os.Getenv("JOBKIT_HOME")
	if dir == "" {
		base, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(base, ".jobkit")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create state dir %s: %w", dir, err)
	}
	return dir, nil
}

func join(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ProfilePath is the master profile YAML location.
func ProfilePath() (string, error) { return join("profile.yaml") }

// SearchesPath is the saved board/search profile YAML location.
func SearchesPath() (string, error) { return join("searches.yaml") }

// CompaniesPath is the hidden-market target company YAML location.
func CompaniesPath() (string, error) { return join("companies.yaml") }

// ContactsPath is the append-only contact/referral ledger.
func ContactsPath() (string, error) { return join("contacts.jsonl") }

// CalibrationPath is the opportunity-ranking calibration YAML location.
func CalibrationPath() (string, error) { return join("calibration.yaml") }

// ClaimsPath is the fact-lock allowlist for generated application material.
func ClaimsPath() (string, error) { return join("claims.yaml") }

// LedgerPath is the append-only application-event ledger.
func LedgerPath() (string, error) { return join("applications.jsonl") }

// InboxPath is the append-only saved-job inbox ledger.
func InboxPath() (string, error) { return join("inbox.jsonl") }

// TelemetryPath is the per-run telemetry log.
func TelemetryPath() (string, error) { return join("telemetry.jsonl") }

// OutDir is where generated artifacts (resumes, letters) land by default.
func OutDir() (string, error) {
	p, err := join("out")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}
