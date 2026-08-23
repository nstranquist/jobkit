---
title: State
description: Files under JOBKIT_HOME.
---

All personal state lives under `JOBKIT_HOME`. The default is `~/.jobkit`. Do not commit that directory.

| File | Kind | Purpose |
|---|---|---|
| `profile.yaml` | YAML | Trusted skills and bullets |
| `eligibility.yaml` | YAML | Hard constraints |
| `searches.yaml` | YAML | Board groups, targets, saved queries |
| `companies.yaml` | YAML | Target companies and ATS slugs |
| `claims.yaml` | YAML | Verified number allowlist |
| `calibration.yaml` | YAML | Learned opportunity weights |
| `inbox.jsonl` | JSONL | Pre-application queue |
| `applications.jsonl` | JSONL | Application events |
| `contacts.jsonl` | JSONL | People and referral state |
| `telemetry.jsonl` | JSONL | Command identity, duration, typed error |
| `out/` | dir | Generated packages |
| `coach/` | dir | Public source, decks, private answers |

YAML files reject unknown fields and trailing documents. JSONL writers take a file lock before each append. New files are private: Unix mode `0600`, Windows ACL for the current user.

Telemetry schema v2 stores an allowlisted command id, success, duration, and typed error class. It does not store arguments, paths, output, or raw errors. Set `JOBKIT_TELEMETRY=off` to disable it.

```sh
jobkit doctor telemetry
jobkit doctor telemetry --migrate
```

Run `--migrate` only after you review the counts. Migration removes legacy arguments and raw errors.
