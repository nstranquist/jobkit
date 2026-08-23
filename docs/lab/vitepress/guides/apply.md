---
title: Apply packages
description: Reviewable packages. No submit.
---

A job description argument can be a file, a supported posting URL, stdin (`-`), or an inbox id.

```sh
jobkit match https://job-boards.greenhouse.io/acme/jobs/123
jobkit apply-plan <inbox-id> --tone warm
jobkit apply https://job-boards.greenhouse.io/acme/jobs/123 --tone warm
```

Output lands in `~/.jobkit/out/<id>/`. A typical package contains the tailored resume, letter, prep sheet, match receipt, eligibility receipt, and the raw job text.

Resume formats are `md`, `txt`, `html`, and `pdf`. PDF export uses headless Chrome. Set `JOBKIT_CHROME_BIN` if detection fails.

Letter tones are `professional`, `warm`, and `direct`. The letter names the top required gap instead of hiding it.

::: tip[Claims gate]
Run `jobkit claims init` from the profile. Then run `jobkit claims check`. Resume, letter, apply, and apply-plan fail if a number is not on the allowlist. Contact details and calendar years are exempt.
:::
