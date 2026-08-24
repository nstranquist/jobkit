---
title: Contract
description: What JobKit does and does not do.
---

JobKit is a local job-search toolkit. It finds roles on public company boards, scores fit against a trusted profile, prepares application packages, and tracks the funnel.

## JobKit does

- Keep candidate state on this computer.
- Score fit from a reviewed profile.
- Prepare packages a human can send.
- Record history in append-only ledgers.

## JobKit does not

- Submit a form.
- Send outreach.
- Scrape LinkedIn, Wellfound, Google Jobs, or Indeed.
- Invent a metric, skill, or claim.
- Publish `~/.jobkit`.

!!! warning

    A human must review every package before any send. JobKit never clicks submit. If `~/.jobkit/claims.yaml` exists, generated numbers must match that allowlist.


!!! note "boundary]"

    `jobkit find` and saved searches query public Greenhouse, Lever, and Ashby APIs only. Use other boards by hand. Apply on the company URL. Tag the lane with `--source`.
