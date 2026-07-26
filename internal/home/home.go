// Package home resolves jobkit's state directory (~/.jobkit by default,
// overridable via JOBKIT_HOME for tests and alternate vaults).
package home

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nstranquist/jobkit/internal/privatefs"
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
	if err := privatefs.EnsureDir(dir); err != nil {
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

// EligibilityPath is the hard-constraint policy kept separate from fit and
// opportunity scoring.
func EligibilityPath() (string, error) { return join("eligibility.yaml") }

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
	if err := privatefs.EnsureDir(p); err != nil {
		return "", err
	}
	return p, nil
}

type PermissionIssue struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Mode uint32 `json:"mode"`
	Want uint32 `json:"want"`
}

type PermissionReport struct {
	Root   string            `json:"root"`
	OK     bool              `json:"ok"`
	Fixed  bool              `json:"fixed"`
	Issues []PermissionIssue `json:"issues,omitempty"`
}

// CheckPermissions audits all existing JobKit state without changing it unless
// fix is explicitly requested. Symlinks fail closed and are never followed.
func CheckPermissions(fix bool) (PermissionReport, error) {
	root, err := Dir()
	if err != nil {
		return PermissionReport{}, err
	}
	report := PermissionReport{Root: root, Fixed: fix}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in JobKit state: %s", path)
		}
		kind := "file"
		want := os.FileMode(privatefs.FileMode)
		if entry.IsDir() {
			kind = "directory"
			want = privatefs.DirMode
		}
		if info.Mode().Perm() != want {
			report.Issues = append(report.Issues, PermissionIssue{Path: path, Kind: kind, Mode: uint32(info.Mode().Perm()), Want: uint32(want)})
			if fix {
				if err := os.Chmod(path, want); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return PermissionReport{}, err
	}
	sort.Slice(report.Issues, func(i, j int) bool { return report.Issues[i].Path < report.Issues[j].Path })
	report.OK = len(report.Issues) == 0 || fix
	return report, nil
}
