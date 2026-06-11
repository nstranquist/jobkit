# Changelog

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
