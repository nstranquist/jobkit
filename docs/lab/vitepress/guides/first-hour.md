---
title: First hour
description: Profile, constraints, discover, prepare.
---

# First hour <Badge type="tip" text="v0.10.1" />

::: info Contract
JobKit finds roles, scores fit, and prepares a package. A human submits it. Claims fail closed.
:::

1. **Profile** — Create or bootstrap the YAML profile. Validate it before any search.
2. **Constraints** — Write hard limits in the eligibility file. Fit and eligibility stay separate.
3. **Discover** — Run a saved search or `find`. Queue matches into the inbox.
4. **Prepare** — Build an apply-plan. Review the package. Then submit by hand.

::: code-group

```sh [bootstrap]
jobkit profile bootstrap --source ~/Downloads/resume.pdf --out auto
jobkit profile validate
```

```sh [eligibility]
jobkit eligibility init --home "St. Louis, MO" \
  --countries "United States,USA,US" --languages English \
  --years 7 --relocation-open
```

```sh [find] {2}
jobkit search init
jobkit find backend go --targets ai-infra --remote \
  --sort opportunity --persona backend-platform \
  --eligibility actionable --inbox
```

```sh [prepare]
jobkit inbox list
jobkit apply-plan <inbox-id> --tone warm
```

:::

::: tip Already reviewed?
Skip bootstrap. Run `jobkit profile validate` and start at discover.
:::

::: warning Eligibility is not fit
Hard limits live in eligibility. Opportunity ranking is a separate score. Do not merge them.
:::

::: details Why four steps
Profile feeds scoring. Constraints drop ineligible roles. Discover fills the inbox. Prepare is the last machine step before a human send.
:::
