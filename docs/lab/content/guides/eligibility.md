---
title: Eligibility
description: Hard constraints stay separate from fit score.
---

Eligibility is not fit. A high keyword score cannot promote an ineligible role into an apply package.

| Class | Meaning | Default search |
|---|---|---|
| `eligible` | The role meets every hard constraint. | Included in `actionable` |
| `review` | A constraint is unknown. Example: relocation needed. | Included in `actionable` |
| `ineligible` | A hard constraint fails. | Excluded |

`apply` and `apply-plan` fail closed on an ineligible role. A human can pass `--override-eligibility`. JobKit records that override on the tracker and in the package receipt.

A user-owned company blacklist (`~/.jobkit/blacklist.yaml`) is the same kind of gate. JobKit never writes names into that file. A hit is `ineligible` with reason `blacklist`. Fit score stays unchanged.
