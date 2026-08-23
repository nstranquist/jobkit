---
title: First hour
description: Profile, constraints, discover, prepare.
---

# First hour

:::note
JobKit finds roles, scores fit, and prepares a package. A human submits it. Claims fail closed.
:::

:::steps

### Profile

Create or bootstrap the YAML profile. Validate it before any search.

```sh
jobkit profile bootstrap --source ~/Downloads/resume.pdf --out auto
jobkit profile validate
```

### Constraints

Write hard limits in the eligibility file. Fit and eligibility stay separate.

```sh
jobkit eligibility init --home "St. Louis, MO" \
  --countries "United States,USA,US" --languages English \
  --years 7 --relocation-open
```

### Discover

Run a saved search or `find`. Queue matches into the inbox.

```sh
jobkit search init
jobkit find backend go --targets ai-infra --remote \
  --sort opportunity --persona backend-platform \
  --eligibility actionable --inbox
```

### Prepare

Build an apply-plan. Review the package. Then submit by hand.

```sh
jobkit inbox list
jobkit apply-plan <inbox-id> --tone warm
```

:::

:::tip
If you already have a reviewed profile, skip bootstrap and run `jobkit profile validate`.
:::

:::warning
Eligibility is not fit. Do not merge the two scores.
:::
