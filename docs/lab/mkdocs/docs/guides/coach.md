---
title: Coach
description: Interview drills from reviewed public evidence.
---

Coach builds role-specific drills from a reviewed public evidence bundle. Private catalog paths are rejected on import.

```sh
# nicos-tools path: check resume facts, then import.
ndev catalog portfolio coach sync

# Standalone path:
jobkit coach source import examples/coach-source.json
jobkit coach deck --job posting.txt --mode mixed --minutes 20
jobkit coach run latest
jobkit coach stats
jobkit coach serve
```

Open the printed loopback URL after `coach serve`. The server binds only to loopback addresses. It uses an ephemeral token and a protected cookie.

| Area | Weight | What JobKit checks |
|---|---:|---|
| Structure | 25 | The answer has the required structure. |
| Decisions and tradeoffs | 25 | Concepts, alternatives, constraints, and risks. |
| Role relevance | 20 | Skills from the target job description. |
| Evidence safety | 20 | Every quantified claim matches an imported claim. |
| Concision | 10 | Length fits the allotted response time. |

!!! warning

    An unsupported quantified claim caps the score below 60. JobKit schedules that answer for review in one day. Model feedback cannot replace the deterministic score. A provider name that ends in `-hosted` sends the practice request and answer text to that provider.

