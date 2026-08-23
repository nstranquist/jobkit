---
title: Weekly loop
description: Monday digest, daily packages, Friday stats.
---

The default slate is 10 roles: 5 platform, 3 full-stack, 1 adoption, 1 stretch. One employer can appear at most twice.

| When | Command | Action |
|---|---|---|
| Monday | `jobkit search digest --inbox --out auto` | Review the digest. Then run `jobkit inbox slate --out auto`. |
| Daily | `jobkit apply-plan <id>` | Review the package. Submit on the company site. Track the send. |
| Daily | `jobkit inbox outreach <id>` | Edit the draft. Send the note yourself. Record the contact. |
| Friday | `jobkit --json track stats` | Compare conversion by `lane` and `source`. |

```sh
jobkit inbox slate --out auto
jobkit inbox stale --days 14
jobkit track followups --days 7
jobkit calibrate report --persona backend-platform
```

::: warning
Do not add a SpaceX-family saved search to the default digest. That board is large and mostly on-site. If relocation is open, run a separate find against the `spacex-family` target.
:::

```sh
jobkit find "software engineer" --targets spacex-family \
  --sort opportunity --persona backend-platform --eligibility actionable
```
