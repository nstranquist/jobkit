// Package telemetry records privacy-bounded CLI outcomes for local self-evaluation.
//
// Records contain an allowlisted command identity, duration, success, and a
// typed error class. They never contain arguments, output, paths, or raw error
// messages. Recording is best-effort and never blocks the command.
package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

const SchemaVersion = 2

type record struct {
	SchemaVersion int       `json:"schema_version"`
	TS            time.Time `json:"ts"`
	Command       string    `json:"command"`
	OK            bool      `json:"ok"`
	DurationMS    int64     `json:"duration_ms"`
	ErrorKind     string    `json:"error_kind,omitempty"`
}

// AuditReport describes telemetry shape without returning telemetry content.
type AuditReport struct {
	Path           string `json:"path"`
	Records        int    `json:"records"`
	CurrentRecords int    `json:"current_records"`
	LegacyRecords  int    `json:"legacy_records"`
	InvalidRecords int    `json:"invalid_records"`
	MigrationReady bool   `json:"migration_ready"`
	Migrated       bool   `json:"migrated,omitempty"`
}

var subcommands = map[string]map[string]bool{
	"profile":     set("path", "show", "validate", "bootstrap"),
	"search":      set("init", "path", "list", "show", "digest", "run"),
	"calibrate":   set("path", "show", "report", "apply"),
	"eligibility": set("init", "path", "show", "check"),
	"doctor":      set("permissions", "telemetry"),
	"claims":      set("path", "init", "show", "check"),
	"company":     set("path", "add", "signal", "note", "show", "list"),
	"contact":     set("path", "add", "import", "list", "show", "touch", "referral", "note"),
	"contacts":    set("path", "add", "import", "list", "show", "touch", "referral", "note"),
	"coach":       set("source", "deck", "run", "stats", "serve", "study", "path"),
	"inbox":       set("add", "recheck", "slate", "list", "stale", "show", "outreach", "form", "set", "note"),
	"track":       set("add", "list", "show", "set", "note", "board", "stats", "followups", "remind"),
}

var rootCommands = set(
	"init", "profile", "search", "calibrate", "eligibility", "doctor", "claims",
	"company", "contact", "contacts", "jd", "find", "match", "resume", "letter",
	"prep", "coach", "apply-plan", "apply", "inbox", "track", "version", "help",
)

func set(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

// CommandID reduces parsed positionals to an allowlisted command identity.
func CommandID(args []string) string {
	if len(args) == 0 {
		return "help"
	}
	root := strings.ToLower(strings.TrimSpace(args[0]))
	if !rootCommands[root] {
		return "unknown"
	}
	if len(args) > 1 && subcommands[root][strings.ToLower(strings.TrimSpace(args[1]))] {
		return root + "." + strings.ToLower(strings.TrimSpace(args[1]))
	}
	return root
}

// Record logs one privacy-bounded run. Errors are swallowed by design.
func Record(command string, start time.Time, runErr error) {
	if os.Getenv("JOBKIT_TELEMETRY") == "off" {
		return
	}
	path, err := home.TelemetryPath()
	if err != nil {
		return
	}
	item := record{
		SchemaVersion: SchemaVersion,
		TS:            time.Now().UTC(),
		Command:       command,
		OK:            runErr == nil,
		DurationMS:    time.Since(start).Milliseconds(),
		ErrorKind:     errorKind(runErr),
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return
	}
	file, err := privatefs.OpenAppend(path)
	if err != nil {
		return
	}
	buffer := append(raw, '\n')
	n, writeErr := file.Write(buffer)
	closeErr := file.Close()
	if writeErr != nil || n != len(buffer) || closeErr != nil {
		return
	}
}

func errorKind(err error) string {
	if err == nil {
		return ""
	}
	var typed *envelope.Err
	if errors.As(err, &typed) && typed.Code != "" {
		return typed.Code
	}
	return envelope.CodeInternal
}

// Audit counts current, legacy, and invalid rows without returning row content.
func Audit(path string) (AuditReport, error) {
	var report AuditReport
	err := privatefs.WithPathLock(path, func() error {
		var auditErr error
		report, auditErr = auditUnlocked(path)
		return auditErr
	})
	return report, err
}

func auditUnlocked(path string) (AuditReport, error) {
	report := AuditReport{Path: path}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		report.MigrationReady = true
		return report, nil
	}
	if err != nil {
		return report, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		report.Records++
		var header struct {
			SchemaVersion int `json:"schema_version"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
			report.InvalidRecords++
		} else if header.SchemaVersion == SchemaVersion {
			var item record
			if err := json.Unmarshal(scanner.Bytes(), &item); err != nil || !validRecord(item) {
				report.InvalidRecords++
			} else {
				report.CurrentRecords++
			}
		} else {
			var legacy legacyRecord
			if err := json.Unmarshal(scanner.Bytes(), &legacy); err != nil || legacy.Cmd == "" {
				report.InvalidRecords++
			} else {
				report.LegacyRecords++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return report, err
	}
	report.MigrationReady = report.InvalidRecords == 0
	return report, nil
}

type legacyRecord struct {
	TS         time.Time `json:"ts"`
	Cmd        string    `json:"cmd"`
	OK         bool      `json:"ok"`
	DurationMS int64     `json:"duration_ms"`
	Err        string    `json:"err,omitempty"`
}

// Migrate atomically replaces a valid telemetry log with schema-v2 records.
// The replacement removes legacy arguments and raw error messages.
func Migrate(path string) (AuditReport, error) {
	return migrate(path, nil)
}

func migrate(path string, beforeReplace func()) (report AuditReport, err error) {
	err = privatefs.WithPathLock(path, func() error {
		var auditErr error
		report, auditErr = auditUnlocked(path)
		if auditErr != nil {
			return auditErr
		}
		if !report.MigrationReady {
			return fmt.Errorf("telemetry contains %d invalid record(s); migration did not change the file", report.InvalidRecords)
		}
		if report.Records == 0 || report.LegacyRecords == 0 {
			report.Migrated = false
			return nil
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		var replacement []byte
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var header struct {
				SchemaVersion int `json:"schema_version"`
			}
			if unmarshalErr := json.Unmarshal(scanner.Bytes(), &header); unmarshalErr != nil {
				_ = file.Close()
				return unmarshalErr
			}
			var item record
			if header.SchemaVersion == SchemaVersion {
				if unmarshalErr := json.Unmarshal(scanner.Bytes(), &item); unmarshalErr != nil {
					_ = file.Close()
					return unmarshalErr
				}
			} else {
				var legacy legacyRecord
				if unmarshalErr := json.Unmarshal(scanner.Bytes(), &legacy); unmarshalErr != nil {
					_ = file.Close()
					return unmarshalErr
				}
				item = record{
					SchemaVersion: SchemaVersion,
					TS:            legacy.TS,
					Command:       CommandID(strings.Fields(legacy.Cmd)),
					OK:            legacy.OK,
					DurationMS:    legacy.DurationMS,
				}
				if !legacy.OK {
					item.ErrorKind = "LEGACY_ERROR"
				}
			}
			raw, marshalErr := json.Marshal(item)
			if marshalErr != nil {
				_ = file.Close()
				return marshalErr
			}
			replacement = append(replacement, raw...)
			replacement = append(replacement, '\n')
		}
		if scanErr := scanner.Err(); scanErr != nil {
			_ = file.Close()
			return scanErr
		}
		if closeErr := file.Close(); closeErr != nil {
			return closeErr
		}
		if beforeReplace != nil {
			beforeReplace()
		}
		if writeErr := privatefs.WriteFile(path, replacement); writeErr != nil {
			return writeErr
		}
		report.CurrentRecords = report.Records
		report.LegacyRecords = 0
		report.Migrated = true
		return nil
	})
	return report, err
}

func validRecord(item record) bool {
	if item.SchemaVersion != SchemaVersion || item.TS.IsZero() || item.DurationMS < 0 {
		return false
	}
	if item.Command == "" || strings.ContainsAny(item.Command, " \t\r\n/\\") {
		return false
	}
	if item.OK {
		return item.ErrorKind == ""
	}
	return item.ErrorKind != ""
}
