# jobkit

A multi-purpose job application & resume toolkit — one Go binary, offline-first,
agent-friendly. Built for software-engineering job hunts but profile/lexicon
driven, so it works for any field you can describe in skills and bullets.

## What it does

| Surface | Verb | What you get |
|---|---|---|
| Master profile | `jobkit init`, `jobkit profile show\|validate\|path` | One YAML source of truth (`~/.jobkit/profile.yaml`) every other command reads |
| JD intelligence | `jobkit jd parse <jd>`, `jobkit jd fetch <url>` | Skills detected against a 150-entry tech lexicon (longest-match, no double counting), weighted by section (requirements 2×, nice-to-have 0.75×, per-skill cap), seniority estimate. Every `<jd>` argument accepts a file, a live posting URL, or `-` for stdin |
| Gap scoring | `jobkit match <jd>` | 0–100 weighted coverage score, matched skills with evidence class (declared/tagged/text), missing skills, tailoring advice |
| Golden path | `jobkit apply <jd>` | One shot: tailored resume + cover letter + interview-prep sheet + match report + raw JD into `~/.jobkit/out/<id>/`, application tracked with its score |
| Tailored resumes | `jobkit resume build <jd> [--format md\|txt\|html]` | Skills reordered and bullets ranked by JD relevance (capped at 4/role); renders Markdown, ATS-safe plain text, or print-ready single-file HTML |
| Cover letters | `jobkit letter build <jd> [--tone professional\|warm\|direct]` | Deterministic draft that weaves your top matched skills with their best supporting bullet — honest about the top required gap |
| Interview prep | `jobkit prep <jd>` | Per-skill deep-dive questions anchored to your own stories, gap-defense bridges, a STAR story bank, questions to ask them |
| Application tracker | `jobkit track add\|list\|show\|set\|note\|board\|stats\|followups` | Append-only JSONL event ledger; funnel board, response-rate stats, follow-up nudges |

## Quick start

```sh
make install            # builds and copies to ~/.local/bin/jobkit
jobkit init             # create ~/.jobkit/profile.yaml, then edit it
jobkit profile validate

# Point any command at a posting URL, a saved file, or stdin:
jobkit match https://job-boards.greenhouse.io/acme/jobs/123
jobkit apply https://job-boards.greenhouse.io/acme/jobs/123 --tone warm
#   → ~/.jobkit/out/acme--role/  (resume.html, letter.txt, prep.md, jd.txt, match.json)
#   → tracked with its match score

jobkit prep posting.txt                 # interview-prep sheet
jobkit track set acme --status applied
jobkit track followups --days 7
```

`make demo` runs the full walkthrough against `examples/` in an isolated
temp state dir.

## Agent contract

Every verb takes `--json` and emits `{ok, data}` or
`{ok:false, error:{code, message, hint}}`. Exit codes: `0` ok, `1` error,
`2` `INVALID_ARGS`, `3` `NOT_FOUND`. Flags may appear anywhere
(`--k v` or `--k=v`). `-` reads the JD from stdin.

Environment:

- `JOBKIT_HOME` — state dir (default `~/.jobkit`): profile, ledger, telemetry, out/
- `JOBKIT_PROFILE` — profile path override (useful for multiple personas)
- `JOBKIT_TELEMETRY=off` — disable per-run telemetry (`telemetry.jsonl`)

## Design notes

- **The profile is the database.** Skills carry levels/years/aliases; bullets
  carry tags. Match evidence is three-tiered: declared skill > bullet tag >
  free-text mention — and the gap report tells you when to promote one.
- **The ledger is history.** `applications.jsonl` is append-only; state is a
  replay. Nothing is ever rewritten, so the funnel stats are auditable.
- **Deterministic over generative.** Resume tailoring and letters are pure
  functions of profile + JD — no API keys, no hallucinated experience. An
  optional LLM polish pass is a planned add-on, not a dependency.
- **Multi-purpose by data.** The lexicon (`internal/jd/lexicon.txt`) is plain
  text (`canonical|aliases|category`); swap or extend it for non-SWE domains.

## Layout

```
cmd/jobkit/          CLI dispatch + usage (incl. the apply golden path)
internal/profile/    master profile schema + starter template
internal/jd/         JD parser + embedded skills lexicon + URL fetch/HTML→text
internal/match/      gap scoring + advice
internal/resume/     tailoring + md/txt/html renderers
internal/letter/     deterministic cover-letter drafts
internal/prep/       interview-prep sheet generator
internal/track/      append-only application ledger + stats
internal/telemetry/  per-run JSONL telemetry (best-effort)
internal/envelope/   {ok, data|error} JSON contract
internal/home/       ~/.jobkit state-dir resolution
```

## Roadmap

- LLM polish pass for letters/summaries (opt-in, keychain-backed key)
- `jobkit track remind` — calendar/notification integration for follow-ups
- PDF export (via headless Chrome print of the HTML renderer)
- Job-board search (`jobkit find`) over public board APIs (Greenhouse/Lever/Ashby)
- Multi-profile personas (`JOBKIT_PROFILE` already supports the mechanics)
