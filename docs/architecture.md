# JobKit architecture decisions

Status: implemented and locally verified on 2026-08-12

## Constraints

JobKit handles sensitive job-search data. It must work without a hosted service,
remain easy to inspect, and expose stable contracts to humans and agents. It
must not submit applications or turn model output into career facts.

These constraints drive the stack. Popularity alone is not a selection rule.

## Decision matrix

| Area | Selected approach | Why it fits | Rejected default |
|---|---|---|---|
| Runtime | Go 1.26.6 and one CLI binary | Fast startup, static typing, simple cross-platform builds, strong standard library, and low deployment overhead | A Node or Python runtime would add an interpreter and a larger dependency tree. Rust would increase implementation cost without a demonstrated runtime need. |
| Human-edited state | YAML through `go.yaml.in/yaml/v3` | Profiles, searches, companies, claims, eligibility, and calibration remain readable. The maintained v3 module supports strict decoding. | JSON is noisy for regular editing. TOML would add another schema without solving a current problem. |
| Machine contracts | Versioned JSON | Agents and provider adapters get explicit, portable schemas and stable error envelopes. | Free-form text is ambiguous. Go-internal types would couple consumers to this repository. |
| Event history | Append-only JSONL | Each application, contact, practice session, and telemetry event can be audited and replayed. One damaged line does not hide the rest of the history. | A database would add migration and operational cost before query volume requires it. Rewriting one JSON document would weaken history and concurrent-write safety. |
| Local web UI | Embedded HTML, CSS, and JavaScript on Go `net/http` | The practice UI ships in the binary and uses the same scoring and storage code as the CLI. There is no package-manager or web-build step. | A SPA framework would duplicate contracts and expand the dependency and supply-chain surface for a small localhost UI. |
| Optional AI | Argument arrays plus versioned JSON on standard input and output | Providers stay replaceable. JobKit does not invoke a shell, embed credentials, or make a model authoritative. | Provider SDKs would couple the core to vendors. Shell command strings would add injection and quoting risk. |
| HTML parsing | `golang.org/x/net/html` | A standards-aware parser is safer than regular expressions for public job pages. | Regular-expression HTML parsing is brittle. A browser runtime is too heavy for the default fetch path. |
| File locking | `golang.org/x/sys/unix` and `golang.org/x/sys/windows` | The Go standard library has no portable advisory file-lock API. A stable sidecar lock serializes JSONL appends and atomic replacement. | Process-local mutexes do not protect two JobKit processes. Silent unlocked writes can corrupt a ledger. |
| Private local state | Unix mode bits and Windows access-control lists | Unix creates `0700` directories and `0600` files. Windows creates a protected access-control list for the current user. The permission doctor checks the platform's real authorization model. | POSIX mode bits do not describe Windows access control. A mode-only check can report a false failure or miss an unsafe Windows access-control list. |

## Dependency policy

The compiled non-standard dependency set is limited to three modules:

- `go.yaml.in/yaml/v3 v3.0.5`
- `golang.org/x/net v0.58.0`
- `golang.org/x/sys v0.47.0`

`tools/license-audit` fails closed when this set, a version, or an upstream
license-file digest changes. `govulncheck` scans reachable Go code. GitHub
Actions use exact commit pins and test Linux, macOS, and Windows.

## Safety boundaries

- YAML readers reject unknown fields and trailing documents. This prevents a
  misspelled policy key from being accepted silently.
- JSONL writers lock a stable sidecar file before one complete event is
  appended. Audit and migration use the same lock.
- New state directories and files are private by default. Unix uses mode bits.
  Windows uses protected access-control lists. Existing legacy state changes
  only when the operator runs the explicit permission repair.
- Coach scoring is deterministic and versioned by rubric. Provider feedback is
  advisory and cannot replace the score.
- Provider commands have argument-count, argument-size, timeout, and output
  limits. Stored failures contain an exit code, not provider standard error.
- The Coach server binds only to loopback addresses. It creates an ephemeral
  access token, exchanges it for an `HttpOnly` and `SameSite=Strict` cookie,
  checks `Host` and `Origin`, and sends a restrictive content security policy.
- Browser audio uses an `AudioWorklet`. Audio stays local unless the operator
  explicitly configures a transcriber with a different boundary.
- Telemetry schema v2 records only an allowlisted command identity, success,
  duration, and typed error class. It never records arguments, paths, output,
  or raw errors. `jobkit doctor telemetry` reports counts only. Migration of a
  valid legacy log requires `jobkit doctor telemetry --migrate`.

## When to revisit the stack

Add a database only when measured state size or query latency makes replay a
real problem. Add a web framework only when the embedded UI develops enough
independent state to justify a separate application. Add a provider SDK only
when a required capability cannot be expressed through the current adapter
contract. Record the measurement and the new boundary before making any of
these changes.

## Local proof

Run these commands from the repository root:

```sh
make verify
make test-race
make vulnerability-check
GOOS=windows GOARCH=amd64 go build ./...
```

These commands prove local source quality. They do not prove a public release,
public CI, real-user adoption, application success, or a job outcome.
