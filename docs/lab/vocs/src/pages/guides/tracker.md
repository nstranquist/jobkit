---
title: Tracker
description: Append-only funnel ledger.
---

The tracker is an append-only JSONL ledger. State is a replay. Events are never rewritten.

Status order: `discovered`, `interested`, `applied`, `screening`, `interview`, `offer`, `accepted`. Terminal status values are `rejected`, `withdrawn`, and `ghosted`.

```sh
jobkit track set <id> --status applied \
  --lane platform --source ats --resume-version v1.9.0
jobkit --json track stats
jobkit track followups --days 7
jobkit track remind --format ics --out auto
```

Pass a nicos-resume package receipt with `--resume-manifest`. JobKit rejects candidate or history manifests, failed gates, and a file whose digest does not match the receipt.

`track stats` groups conversion by tag. `lane` and `source` are the useful Friday cohorts.
