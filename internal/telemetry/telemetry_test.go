package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstranquist/jobkit/internal/privatefs"
)

func TestCommandIDOmitArguments(t *testing.T) {
	if got := CommandID([]string{"find", "secret company query"}); got != "find" {
		t.Fatalf("CommandID = %q", got)
	}
	if got := CommandID([]string{"coach", "serve", "private-path"}); got != "coach.serve" {
		t.Fatalf("CommandID = %q", got)
	}
	if got := CommandID([]string{"unknown-command", "secret"}); got != "unknown" {
		t.Fatalf("CommandID = %q", got)
	}
}

func TestMigrateRemovesLegacyArgumentsAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	legacy := "{\"ts\":\"2026-08-12T12:00:00Z\",\"cmd\":\"find secret-company\",\"ok\":false,\"duration_ms\":12,\"err\":\"private/path failed\"}\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Migrate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Migrated || report.LegacyRecords != 0 || report.CurrentRecords != 1 {
		t.Fatalf("report = %#v", report)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, forbidden := range []string{"secret-company", "private/path", "\"cmd\"", "\"err\""} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("migrated telemetry contains %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "\"command\":\"find\"") || !strings.Contains(body, "\"error_kind\":\"LEGACY_ERROR\"") {
		t.Fatalf("migrated telemetry = %s", body)
	}
}

func TestAuditRefusesInvalidMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(path); err == nil {
		t.Fatal("Migrate succeeded for invalid telemetry")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "{not-json}\n" {
		t.Fatalf("invalid telemetry changed: %q, %v", raw, err)
	}
}

func TestMigrateSerializesConcurrentAppendAcrossAtomicReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	legacy := "{\"ts\":\"2026-08-12T12:00:00Z\",\"cmd\":\"find private-query\",\"ok\":true,\"duration_ms\":12}\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	paused := make(chan struct{})
	resume := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(resume)
		}
	}()
	migrateDone := make(chan error, 1)
	go func() {
		_, err := migrate(path, func() {
			close(paused)
			<-resume
		})
		migrateDone <- err
	}()
	<-paused

	appendStarted := make(chan struct{})
	appendDone := make(chan error, 1)
	go func() {
		close(appendStarted)
		file, err := privatefs.OpenAppend(path)
		if err != nil {
			appendDone <- err
			return
		}
		item := record{SchemaVersion: SchemaVersion, TS: time.Now().UTC(), Command: "find", OK: true, DurationMS: 1}
		raw, err := json.Marshal(item)
		if err == nil {
			_, err = file.Write(append(raw, '\n'))
		}
		appendDone <- errorsJoin(err, file.Close())
	}()
	<-appendStarted
	select {
	case err := <-appendDone:
		t.Fatalf("append completed while migration held the path lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(resume)
	released = true
	if err := <-migrateDone; err != nil {
		t.Fatal(err)
	}
	if err := <-appendDone; err != nil {
		t.Fatal(err)
	}
	report, err := Audit(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Records != 2 || report.CurrentRecords != 2 || report.LegacyRecords != 0 || report.InvalidRecords != 0 {
		t.Fatalf("report after concurrent migration and append = %#v", report)
	}
}

func errorsJoin(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
