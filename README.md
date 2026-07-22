# jobkit

A multi-purpose job application & resume toolkit — one Go binary, offline-first,
agent-friendly. Built for software-engineering job hunts but profile/lexicon
driven, so it works for any field you can describe in skills and bullets.

## Showcase

<img src="assets/brand/jobkit.svg" width="96" height="96" alt="JobKit application icon">

![JobKit generated ATS-safe tailored resume](portfolio/assets/tailored-resume.png)

This is real HTML output generated from the repository's fictional example
profile and job description, with no personal data. It is the reviewed evidence
declared in `portfolio/manifest.yaml`.

## What it does

| Surface | Verb | What you get |
|---|---|---|
| Master profile | `jobkit init`, `jobkit profile show\|validate\|path`, `jobkit profile bootstrap --source resume.pdf` | One YAML source of truth (`~/.jobkit/profile.yaml`) every other command reads; bootstrap can draft it from trusted local resume text/PDF |
| Saved searches | `jobkit search init\|list\|show\|run\|digest`, `jobkit find ... --save NAME` | Editable `~/.jobkit/searches.yaml` with board groups, curated `#target` packs, repeatable search profiles, and digest runs |
| Hidden market | `jobkit company add\|signal\|list\|show`, `jobkit contact add\|touch\|referral\|import` | Target-company intelligence, fresh hiring signals, recruiter/contact CRM, referral state, and LinkedIn connections-export bulk import with target-company overlap detection |
| JD intelligence | `jobkit jd parse <jd>`, `jobkit jd fetch <url>` | Skills detected against a 150-entry tech lexicon (longest-match, no double counting), weighted by section (requirements 2×, nice-to-have 0.75×, per-skill cap), seniority estimate. Every `<jd>` argument accepts a file, a live posting URL, or `-` for stdin |
| Job search | `jobkit find <query> --targets ai-infra [--sort opportunity] [--inbox]` | Queries public company job-board APIs (Greenhouse, Lever, Ashby), normalizes title/location/apply URLs, filters by query/location/remote/limit/min-comp, scores compensation/freshness/saturation/persona fit, warns on skipped boards by default, and can queue deduped results |
| Ranking calibration | `jobkit calibrate report\|apply [--persona NAME]` | Learns opportunity-score weights from inbox/tracker outcomes, writes `~/.jobkit/calibration.yaml`, and automatically feeds calibrated weights back into `find`, `search run`, and `search digest` |
| Inbox queue | `jobkit inbox add\|list\|show\|stale\|set\|note\|outreach\|form` | Pre-application queue with dedupe IDs, fit scores, source provenance, last-seen state, outreach drafts, form-fill packets (`--format js` emits a paste-in-DevTools autofill snippet that never submits), statuses, and next actions |
| Claims gate | `jobkit claims init\|check\|show\|path` | Fact lock for generated material: an allowlist of verified quantified claims (`~/.jobkit/claims.yaml`); when present, resume/letter/apply generation fails closed on any number it can't trace to a verified claim |
| Gap scoring | `jobkit match <jd>` | 0–100 weighted coverage score, matched skills with evidence class (declared/tagged/text), missing skills, tailoring advice |
| Human-in-loop plan | `jobkit apply-plan <jd\|url\|inbox-id>` | Reviewable package with resume, letter, prep sheet, match report, raw JD, and `plan.md` checklist; no form auto-submit |
| Golden path | `jobkit apply <jd>` | One shot: tailored resume + cover letter + interview-prep sheet + match report + raw JD into `~/.jobkit/out/<id>/`, application tracked with its score |
| Tailored resumes | `jobkit resume build <jd> [--format md\|txt\|html\|pdf]` | Skills reordered and bullets ranked by JD relevance (capped at 4/role); renders Markdown, ATS-safe plain text, print-ready single-file HTML, or PDF via headless Chrome |
| Cover letters | `jobkit letter build <jd> [--tone professional\|warm\|direct]` | Deterministic draft that weaves your top matched skills with their best supporting bullet — honest about the top required gap |
| Interview prep | `jobkit prep <jd>` | Per-skill deep-dive questions anchored to your own stories, gap-defense bridges, a STAR story bank, questions to ask them |
| Application tracker | `jobkit track add\|list\|show\|set\|note\|board\|stats\|followups\|remind` | Append-only JSONL event ledger; funnel board, response-rate stats with per-tag conversion breakdowns (`--resume-version`, `--lane`, `--source`, `--tag k=v`), follow-up nudges, text/ICS reminder export |

## Quick start

```sh
make install            # builds and copies to ~/.local/bin/jobkit
jobkit profile bootstrap --source ~/Downloads/resume.pdf --out auto
# or: jobkit init     # create ~/.jobkit/profile.yaml, then edit it
jobkit profile validate

# Saved searches + inbox:
jobkit search init
jobkit find backend go --targets ai-infra --remote --sort opportunity --persona agent-infra --min-comp 250000 --save backend-go --inbox
jobkit search run backend-go --inbox
jobkit search digest --inbox --out auto
jobkit inbox list
jobkit inbox stale --days 14
jobkit calibrate report --persona agent-infra
jobkit calibrate apply --persona agent-infra --min-samples 8
jobkit company add "Modal" --domain modal.com --tags ai,infra --boards ashby:Modal --target-comp 400000
jobkit company signal "Modal" --type team-growth --source linkedin --note "infra hiring push"
jobkit contact add "Jane Recruiter" --company Modal --channel linkedin --source search --inbox-id <inbox-id>
jobkit contact referral modal --status referral-requested --note "asked for routing advice" --track-id <track-id>

# Point any JD command at a posting URL, a saved file, an inbox id, or stdin:
jobkit match https://job-boards.greenhouse.io/acme/jobs/123
jobkit apply-plan <inbox-id> --tone warm
jobkit inbox outreach <inbox-id> --channel linkedin --out auto
jobkit inbox form <inbox-id> --out auto
jobkit apply https://job-boards.greenhouse.io/acme/jobs/123 --tone warm
#   → ~/.jobkit/out/acme--role/  (resume.html, letter.txt, prep.md, jd.txt, match.json)
#   → tracked with its match score

jobkit prep posting.txt                 # interview-prep sheet
jobkit track set acme --status applied
jobkit track followups --days 7
jobkit track remind --format ics --out auto
```

`make demo` runs the full walkthrough against `examples/` in an isolated
temp state dir.

## Publication checks

Local publication gate (no remote push):

```sh
make publish-ready   # alias: make verify-publication
make demo            # isolated JOBKIT_HOME; uses examples/ only
```

`make verify` runs gofmt, the test suite, `go vet`, and the deterministic
dependency license audit. The audit fails closed if a compiled non-standard
dependency is not allowlisted or if its upstream license text changes.
`LICENSE` covers Nico-authored JobKit code; `THIRD_PARTY_NOTICES.md` records
dependency rights. Personal JobKit state under `~/.jobkit` is never part of a
source publication — only synthetic `examples/` fixtures ship.

## Agent contract

Every verb takes `--json` and emits `{ok, data}` or
`{ok:false, error:{code, message, hint}}`. Exit codes: `0` ok, `1` error,
`2` `INVALID_ARGS`, `3` `NOT_FOUND`. Flags may appear anywhere
(`--k v` or `--k=v`). `-` reads the JD from stdin.

For agent context windows prefer **`jobkit help --compact`** (or
`jobkit help --json --compact`) instead of the full human usage dump.

Job search is partial-result tolerant by default: if one board in a group is
stale or unavailable, text mode prints a warning to stderr and JSON mode
includes `warnings` while returning successful-board results. Pass `--strict`
to fail the whole search on the first board error.

Search can be ranked by opportunity signals: `--sort opportunity|comp|freshness`
and `--min-comp N` use extracted salary ranges when postings include them;
`--persona NAME` biases the opportunity score toward a search persona such as
`agent-infra`, `ai-product`, `devtools`, or `backend-platform`. Once
`jobkit calibrate apply` writes `~/.jobkit/calibration.yaml`, all public-board
search ranking uses those active weights automatically.

Environment:

- `JOBKIT_HOME` — state dir (default `~/.jobkit`): profile, searches,
  companies, calibration, ledgers, telemetry, out/
- `JOBKIT_PROFILE` — profile path override (useful for multiple personas)
- `JOBKIT_TELEMETRY=off` — disable per-run telemetry (`telemetry.jsonl`)
- `JOBKIT_CHROME_BIN` — explicit Chrome/Chromium binary for PDF export
- `JOBKIT_GREENHOUSE_BASE`, `JOBKIT_LEVER_BASE`, `JOBKIT_ASHBY_BASE` — test/dev
  overrides for public board API base URLs

## Design notes

- **The profile is the database.** Skills carry levels/years/aliases; bullets
  carry tags. Match evidence is three-tiered: declared skill > bullet tag >
  free-text mention — and the gap report tells you when to promote one. The
  one-way, SemVer'd consumer boundary is documented in
  [`contracts/profile/`](contracts/profile/); consumers validate the synthetic
  fixture instead of importing JobKit's internal Go package.
- **The ledger is history.** `applications.jsonl` and `contacts.jsonl` are
  append-only; state is a replay. Nothing is ever rewritten, so the funnel,
  follow-up, and referral trails are auditable.
- **Hidden market means dated signals.** `companies.yaml` is intentionally
  human-editable: public ATS boards, target compensation, tags, and hiring
  signals combine into a next-action score so outreach can happen before a
  posting is everywhere.
- **Opportunity beats raw keyword match.** Public board results carry
  compensation, freshness, saturation, and persona scores. The default weights
  are useful immediately; `jobkit calibrate apply` tunes them against observed
  inbox/tracker outcomes as the hunt produces signal.
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
internal/jobsearch/  Greenhouse/Lever/Ashby search + opportunity ranking
internal/calibration/ outcome-based opportunity weight calibration
internal/searches/   saved board groups + search profiles
internal/company/    hidden-market target-company intelligence
internal/contacts/   relationship/referral append-only ledger
internal/inbox/      pre-application saved-job queue
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
- Native reminder delivery to macOS Reminders/Calendar (current `track remind`
  exports text or `.ics`)
- Browser-assisted form fill with explicit human approval at each page boundary
- Scheduled digest/target-company refresh automation
- Multi-profile personas (`JOBKIT_PROFILE` already supports the mechanics)
