# Changelog

## [0.9.0] — 2026-08-12

- **Evidence-linked interview coach** — `jobkit coach source|deck|run|stats`
  validates public project evidence, builds role-specific practice decks,
  scores answers, and schedules reviews after 1, 3, 7, or 14 days.
- **Claim-safe scoring** — unsupported quantified claims cap an answer below
  60. The session records each violation and schedules a one-day review.
- **Provider-neutral feedback** — optional adapters use versioned JSON on
  standard input and output. JobKit executes an argument array without a shell.
  Provider errors do not replace or block deterministic scoring.
- **Localhost practice UI** — `jobkit coach serve` binds to loopback addresses
  only. The UI uses the same deck, scoring, session, and statistics engine.
  A configured local transcriber can add microphone answers.
- **Private, append-only practice state** — source bundles, decks, and sessions
  use restrictive local permissions under `~/.jobkit/coach`.
- **Stale-source and answer gates** — scoring rejects a deck after its source
  changes. It also rejects missing, duplicate, and unknown question IDs.
- **Provider selection in the localhost UI** — configured advisory providers
  are available beside the deck selector. Provider errors stay supplemental
  and never replace the deterministic score.
- **Authenticated localhost UI** — the server creates an ephemeral access
  token, exchanges it for a protected local cookie, validates host and origin,
  and serves embedded assets under a restrictive content security policy.
- **Versioned scoring proof** — sessions record rubric `1.0.0`; statistics keep
  rubric versions separate; benchmark fixtures detect scoring drift.
- **Privacy-bounded telemetry** — schema v2 stores allowlisted command IDs and
  typed errors only. `jobkit doctor telemetry` audits counts, and `--migrate`
  explicitly removes legacy arguments and raw errors.
- **Strict and concurrent state handling** — all YAML authorities reject
  unknown fields and trailing documents. Cross-platform file locks protect
  append-only event writes from concurrent processes.
- **Safe telemetry replacement** — a stable path lock now serializes telemetry
  audit and migration with append operations. Atomic replacement cannot discard
  an event that another JobKit process appends concurrently.
- **Exact claim shapes** — money, percentages, magnitude values, lower bounds,
  and bare counts no longer authorize one another when their digits match.
- **Dependency and CI hardening** — Go 1.26.5, maintained YAML v3, exact action
  pins, Linux/macOS/Windows tests, reachable-code vulnerability scanning, and
  a three-module fail-closed license allowlist.
- **Bounded provider execution** — provider argument and output sizes are
  capped, timeouts stay mandatory, and persisted failures omit raw standard
  error text.
- **Maintained claims-gate integration** — the publication fixture prefers
  `agent-ops claims check`, with the deprecated standalone `claimguard` binary
  retained only as a compatibility fallback.

## [0.8.0] — 2026-07-22

- **Eligibility as an independent signal** — `jobkit eligibility
  init|show|path|check` stores an explainable hard-constraint policy in
  `~/.jobkit/eligibility.yaml` and classifies geography, required language,
  years, work mode, travel, role family, management, and sales as
  `eligible|review|ineligible` without contaminating fit or opportunity scores.
- **Fail-closed application packages** — `find` and saved searches default to
  actionable roles when a policy exists; inbox items retain assessments and
  reason codes; `apply`/`apply-plan` block ineligible jobs unless a human uses
  `--override-eligibility`, which is recorded in the receipt and tracker.
- **Resume artifact provenance** — `track add`/`track set` can ingest a
  verified nicos-resume package manifest and persist its variant, selected
  artifact SHA-256, source digest, and claim-set identity. Candidate/history
  manifests, incomplete or failed gate sets, and upload files that do not match
  the selected manifest digest are rejected; JobKit-generated packages record
  their own artifact digest, tailoring receipt, and structured eligibility
  override.
- **Repeatable weekly mix** — `jobkit inbox slate` selects five
  platform/DevEx/AI-infrastructure, three full-stack product, one technical
  adoption/FDE, and one stretch role by default, with at most two roles per
  employer. It reports lane shortages rather than silently changing the mix.
- **Append-only reassessment** — `jobkit inbox recheck` applies the current
  policy to existing saved jobs through new ledger events, preserving history.
- **Honest interview bridges** — prep sheets now use compatible skill families
  instead of treating every broad "domain" skill as interchangeable, and can
  anchor technical questions/STAR stories in project bullets as well as job
  experience.
- **SemVer minor release** — CLI and public-board User-Agent report `0.8.0`;
  compact/full help and publication documentation cover the new safety gates.

## [0.7.1] — 2026-07-20

- **Compact help for agents** — `jobkit help --compact` (and
  `jobkit help --json --compact`) emits a token-efficient verb map (~13% of
  full help) while full `jobkit help` remains the human reference.
- **Letter package tests** — deterministic cover-letter tones, manager
  greeting, required-gap honesty, and headline pipe-segment trimming.
- **Publication tooling** — `.github/workflows/ci.yml` runs
  `make verify-publication` plus a dedicated `make test-race` job; Makefile
  gains `fmt`, `test-race`, `publish-ready`, and gitleaks gates;
  `CONTRIBUTING.md` + `SECURITY.md` document local boundaries.
- **Honesty nits** — public board User-Agent reports `jobkit/0.7.1`; README
  documents `make publish-ready` and synthetic-fixture-only publication.

## [0.7.0] — 2026-07-16

- **Claims gate (fact lock for generated material)** — new `jobkit claims
  init|check|show|path` maintains an allowlist of verified quantified claims
  (`~/.jobkit/claims.yaml`). When the file exists, `resume build`,
  `letter build`, `apply`, and `apply-plan` fail closed on any quantified
  claim (percentages, `N+` figures, money, years, large numbers) not covered
  by the allowlist, reporting each violating token with context. Contact
  details (phones, emails, URLs) and calendar years are exempt. `claims init`
  bootstraps entries from the profile for human curation.
- **Funnel tags on the tracker** — `track add`/`track set` accept
  `--resume-version`, `--lane`, `--source`, and generic `--tag k=v[,k=v]`;
  tags merge on replay (later events win) and `track stats` now breaks down
  applied/responded/interview conversion per tag value, so lane and resume
  version performance is measurable from real outcomes.
- **LinkedIn contacts import** — `contact import <connections.csv>` parses
  the LinkedIn data-export CSV (including its "Notes:" preamble), dedupes
  against the existing ledger by name+company and profile URL, tags rows
  `source=linkedin-export`, and reports contacts at tracked target companies
  as warm-referral candidates.
- **Form-fill autofill snippet** — `inbox form <id> --format js` emits a
  self-contained paste-in-DevTools script that fills standard Greenhouse,
  Lever, and generic label-matched application fields from the profile
  (React-safe value setting), highlights everything it filled, and by
  design never touches uploads, custom questions, EEO fields, or submit.
- **Recall-safe query matching** — `find`/saved-search queries of four or
  more terms now require 60% term coverage instead of every term, so one
  rare word (e.g. "backstage") can't zero a result set; queries of up to
  three terms remain strict. Scoring still rewards full matches.

## [0.6.0] — 2026-06-24

- **Outcome-calibrated opportunity ranking** — `jobkit calibrate report|apply`
  joins inbox jobs with tracker outcomes, evaluates positive-vs-negative
  pairwise ranking accuracy, and writes active weights to
  `~/.jobkit/calibration.yaml`.
- **Search uses active calibration** — `find`, `search run`, and
  `search digest` keep the default opportunity formula when no calibration
  exists, and automatically use calibrated weights once applied.
- **Inbox score provenance** — saved public-board jobs now retain compensation
  and opportunity score components so later calibration can learn from the
  exact search signals that were shown at discovery time.
- **Calibration regression coverage** — tests cover tracker-over-inbox outcome
  precedence, YAML round-trip, weighted scoring, and the CLI `calibrate apply`
  path.

## [0.5.0] — 2026-06-24

- **Compensation-aware search** — public-board results now extract salary
  ranges from posting text and Ashby's structured `jobs[].compensation` payload;
  `find`/saved searches support `--min-comp` and `--sort comp`.
- **Opportunity-ranked search** — `find`, `search run`, and `search digest`
  now carry compensation extraction, freshness, saturation risk, persona scores,
  `--sort opportunity|comp|freshness`, `--persona`, and `--min-comp`.
- **Saved-search ranking profiles** — saved searches persist `sort`, `persona`,
  and `min_comp`, so digest runs keep using the higher-signal ranking mode.
- **Hidden-market companies** — `jobkit company add|signal|list|show|note`
  tracks target companies, public ATS boards, target compensation, and dated
  hiring signals in `~/.jobkit/companies.yaml`.
- **Contacts/referrals ledger** — `jobkit contact add|list|show|touch|referral|note`
  stores recruiter, hiring-manager, and referral relationships in append-only
  `~/.jobkit/contacts.jsonl`, linkable to inbox items and tracker IDs.

## [0.4.0] — 2026-06-24

- **Saved searches** — `jobkit search init|list|show|run` manages editable
  board groups and saved query profiles in `~/.jobkit/searches.yaml`; `find`
  can use `@group` board refs and `--save NAME`.
- **Target packs + digests** — searches can use curated `#target` packs,
  `find --targets NAME`, and `search digest [name]` for markdown/JSON digest
  runs that optionally refresh inbox state.
- **Partial search results** — `find` and `search run` now keep successful board
  results when another board fails, emitting warnings by default; `--strict`
  restores fail-fast behavior.
- **Inbox queue** — `jobkit find ... --inbox` and `jobkit inbox add|list|show|set|note`
  create a deduped pre-application queue with fit scores and next actions.
- **Provenance + stale detection** — inbox jobs now track source query, board,
  fingerprint, first/last seen time, repeated sightings, and `inbox stale`.
- **Outreach/form packets** — `inbox outreach`, `inbox form`, and `apply-plan`
  produce human-reviewed outreach and form-fill artifacts without submission.
- **Human-in-loop apply plans** — `jobkit apply-plan <jd|url|inbox-id>` writes
  resume, letter, prep, JD, match report, and `plan.md` checklist without
  submitting forms; it tracks the lead as `interested`.
- **Profile bootstrap** — `jobkit profile bootstrap --source resume.pdf` drafts
  a `profile.yaml` from trusted text/PDF resume sources using local extraction.

## [0.3.0] — 2026-06-17

- **`jobkit find <query> --boards ...`** — searches public company board APIs
  for Greenhouse, Lever, and Ashby; accepts `provider:slug` specs or hosted
  board URLs; supports `--remote`, `--location`, `--limit`, and `--json`.
- **PDF export** — `resume build --format pdf --out <path|auto>` and
  `apply --format pdf` render through the existing print-ready HTML path via
  headless Chrome/Chromium. `JOBKIT_CHROME_BIN` pins the browser binary for
  deterministic environments.
- **`track remind`** — exports due follow-up reminders as text, `.ics`, or JSON
  without mutating the append-only ledger.

## [0.2.0] — 2026-06-11

- **`jobkit apply <jd>`** — golden path: tailored resume + cover letter +
  prep sheet + match report + raw JD written to `~/.jobkit/out/<id>/`, and the
  application tracked with its score in one shot.
- **URL ingestion** — every `<jd>` argument now accepts a posting URL;
  `jobkit jd fetch <url>` downloads + converts HTML to clean, section-preserving
  text (verified live against a Greenhouse posting).
- **`jobkit prep <jd>`** — interview-prep sheet: per-skill deep-dive questions
  anchored to your own bullets, gap-defense bridges from the nearest declared
  skill in the same category, STAR story bank, questions to ask them.
- **Scoring fixes (found by dogfooding):**
  - Longest-match span dedup — one "gRPC" mention no longer counts twice
    (canonical + alias), "ruby on rails" no longer also credits Ruby/Rails
    separately, "react native" no longer credits React.
  - Per-skill weight cap (6.0) — a term repeated 20× (e.g. a company name in
    page boilerplate) can't dominate the coverage denominator.
  - `GitLab CI` lexicon alias tightened (bare `gitlab` matched the company name).
- `track list`/`followups` `--json` now emit `[]` instead of `null` when empty.
- Letter no longer lowercases the headline ("a senior Software Engineer").

## [0.1.0] — 2026-06-11

Initial release: master profile, JD parsing (150-entry lexicon,
section-weighted), gap scoring with evidence classes, tailored resume
renderers (md / ATS txt / print HTML), deterministic cover letters,
append-only JSONL application tracker (board/stats/followups), agent JSON
envelope, per-run telemetry.
