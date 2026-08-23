---
title: First hour
description: Profile, constraints, discover, prepare.
---

1. **Profile** — Create or bootstrap the YAML profile. Validate it before any search.
2. **Constraints** — Write hard limits in the eligibility file. Fit and eligibility stay separate.
3. **Discover** — Run a saved search or `find`. Queue matches into the inbox.
4. **Prepare** — Build an apply-plan. Review the package. Then submit by hand.

```sh
jobkit profile bootstrap --source ~/Downloads/resume.pdf --out auto
jobkit profile validate

jobkit eligibility init --home "St. Louis, MO" \
  --countries "United States,USA,US" --languages English \
  --years 7 --relocation-open

jobkit search init
jobkit find backend go --targets ai-infra --remote \
  --sort opportunity --persona backend-platform \
  --eligibility actionable --inbox

jobkit inbox list
jobkit apply-plan <inbox-id> --tone warm
```

If you already have a reviewed profile, skip bootstrap and run `jobkit profile validate`.
