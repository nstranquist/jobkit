# Contributing to JobKit

Local development for the offline-first job application toolkit.

## Setup

```sh
go test ./...
make test-race     # optional locally; also a CI job
make verify
make publish-ready # full publication gate (verify + gitleaks)
make demo          # isolated JOBKIT_HOME; never reads ~/.jobkit
```

Requires Go 1.26.6. CI tests Linux, macOS, and Windows with that patch release.

## Agent-friendly contract

- Every verb accepts `--json` and emits `{ok, data}` or
  `{ok:false, error:{code,message,hint}}`.
- Prefer `jobkit help --compact` in agent context windows; full `jobkit help`
  for humans.
- Personal state lives under `JOBKIT_HOME` (default `~/.jobkit`). **Never**
  commit real profiles, applications, contacts, or claims.

## Publication boundary

- `make verify-publication` is the single local gate (tests, vet, license
  audit, gitleaks history + tree).
- Synthetic fixtures only: `examples/profile.yaml`, `examples/jd-backend.txt`.
- Public push remains human-gated; this repo may be private or local-only.

## Layout

| Path | Role |
| --- | --- |
| `cmd/jobkit` | CLI dispatch |
| `internal/*` | Domain packages (match, resume, letter, track, …) |
| `examples/` | Synthetic demo fixtures |
| `tools/license-audit` | Fail-closed dependency license gate |
| `docs/case-study.md` | Recruiter-readable narrative |

## Style

- Deterministic generation over LLM drafts for core artifacts.
- Append-only ledgers for history (`applications.jsonl`, `contacts.jsonl`).
- Prefer table-driven Go tests next to the package under test.
