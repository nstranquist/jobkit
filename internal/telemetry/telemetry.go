// Package telemetry appends one JSONL record per CLI run to
// ~/.jobkit/telemetry.jsonl — the self-eval substrate (which verbs get used,
// what fails). Best-effort: never blocks or fails the command.
package telemetry

import (
	"encoding/json"
	"os"
	"time"

	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

type record struct {
	TS         time.Time `json:"ts"`
	Cmd        string    `json:"cmd"`
	OK         bool      `json:"ok"`
	DurationMS int64     `json:"duration_ms"`
	Err        string    `json:"err,omitempty"`
}

// Record logs one run. Errors are swallowed by design.
func Record(cmd string, start time.Time, runErr error) {
	if os.Getenv("JOBKIT_TELEMETRY") == "off" {
		return
	}
	path, err := home.TelemetryPath()
	if err != nil {
		return
	}
	r := record{TS: time.Now().UTC(), Cmd: cmd, OK: runErr == nil, DurationMS: time.Since(start).Milliseconds()}
	if runErr != nil {
		r.Err = runErr.Error()
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return
	}
	f, err := privatefs.OpenAppend(path)
	if err != nil {
		return
	}
	buf := append(raw, '\n')
	n, writeErr := f.Write(buf)
	closeErr := f.Close()
	if writeErr != nil || n != len(buf) || closeErr != nil {
		return
	}
}
