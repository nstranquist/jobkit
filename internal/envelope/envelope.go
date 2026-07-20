// Package envelope renders the agent-friendly JSON contract shared by all
// jobkit verbs: {ok, data} on success, {ok:false, error:{code,message,hint}}
// on failure, with stable exit codes per error class.
package envelope

import (
	"encoding/json"
	"fmt"
	"os"
)

// Error codes with stable exit-code mapping.
const (
	CodeInvalidArgs = "INVALID_ARGS" // exit 2
	CodeNotFound    = "NOT_FOUND"    // exit 3
	CodeIOFailed    = "IO_FAILED"    // exit 1
	CodeInternal    = "INTERNAL"     // exit 1
)

// Err is a typed CLI error carrying an envelope code and optional hint.
type Err struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Err) Error() string { return e.Message }

// New builds a typed error.
func New(code, message string) *Err { return &Err{Code: code, Message: message} }

// Newf builds a typed error with formatting.
func Newf(code, format string, args ...any) *Err {
	return &Err{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithHint attaches a remediation hint.
func (e *Err) WithHint(hint string) *Err { e.Hint = hint; return e }

// ExitCode maps an error to its contract exit code.
func ExitCode(err error) int {
	if e, ok := err.(*Err); ok {
		switch e.Code {
		case CodeInvalidArgs:
			return 2
		case CodeNotFound:
			return 3
		}
	}
	return 1
}

type success struct {
	OK   bool `json:"ok"`
	Data data `json:"data"`
}

type data any

type failure struct {
	OK    bool `json:"ok"`
	Error *Err `json:"error"`
}

// EmitData writes a success envelope to stdout.
func EmitData(payload data) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(success{OK: true, Data: payload}); err != nil {
		fmt.Fprintf(os.Stderr, "jobkit: encode success envelope: %v\n", err)
	}
}

// EmitError writes a failure envelope to stdout (so agents always parse one
// stream) and returns the exit code.
func EmitError(err error) int {
	e, ok := err.(*Err)
	if !ok {
		e = &Err{Code: CodeInternal, Message: err.Error()}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(failure{OK: false, Error: e}); err != nil {
		fmt.Fprintf(os.Stderr, "jobkit: encode error envelope: %v\n", err)
	}
	return ExitCode(e)
}
