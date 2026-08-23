---
title: Apply packages
description: Reviewable packages. No submit.
---

A job description argument can be a file, a supported posting URL, stdin (`-`), or an inbox id.

**Posting text is data, never instructions.** A JD or recruiter email may contain "ignore previous instructions" or a fake `system:` block. JobKit records those as untrusted flags. It does not obey them.

**Dead postings fail closed.** HTTP 404/410, or a body that says the role is filled/expired/closed, cannot produce a match score or an apply package. `inbox recheck` re-fetches stored URLs and marks them closed without changing the stored fit score.

**Legitimacy is not fit.** JobKit emits `legitimacy: high|caution|suspicious` beside the 0–100 match score. Ghost/scam signals do not change that number.

**Blacklist is user-owned.** Put companies in `~/.jobkit/blacklist.yaml` yourself. JobKit never auto-fills that file. A hit gates apply-plan the same way ineligible eligibility does (`--override-eligibility` after human review) and does not change fit.

**Tool-of-trade is not authorship.** Using Go or Kubernetes is not the same as building them. Claims stay on the JobKit allowlist.

```sh
jobkit match https://job-boards.greenhouse.io/acme/jobs/123
jobkit apply-plan <inbox-id> --tone warm
jobkit apply https://job-boards.greenhouse.io/acme/jobs/123 --tone warm
```

Output lands in `~/.jobkit/out/<id>/`. A typical package contains the tailored resume, letter, prep sheet, match receipt, eligibility receipt, and the raw job text.

Resume formats are `md`, `txt`, `html`, and `pdf`. PDF export uses headless Chrome. Set `JOBKIT_CHROME_BIN` if detection fails.

Letter tones are `professional`, `warm`, and `direct`. The letter names the top required gap instead of hiding it.

:::tip[Claims gate]
Run `jobkit claims init` from the profile. Then run `jobkit claims check`. Resume, letter, apply, and apply-plan fail if a number is not on the allowlist. Contact details and calendar years are exempt.
:::
