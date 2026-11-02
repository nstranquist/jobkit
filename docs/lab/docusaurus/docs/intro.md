---
title: Overview
sidebar_position: 0
---

# JobKit

Local-first job-search toolkit. v0.10.0.

Finds roles, scores fit, prepares packages, and tracks the funnel. It does **not** submit applications. It does **not** invent experience.

```mermaid
flowchart LR
  profile[Profile + eligibility] --> find[find / digest]
  find --> inbox[Inbox slate]
  inbox --> plan[apply-plan]
  plan --> human[Human submit]
  human --> track[track stats]
  track --> cal[calibrate]
  cal --> find
```

:::warning Human review
A human must review every package before any send. JobKit never clicks submit.
:::

:::info Search boundary
`jobkit find` queries public Greenhouse, Lever, and Ashby APIs only.
:::
