---
title: Search
description: Public Greenhouse, Lever, and Ashby boards only.
---

Name a board as `provider:slug`. Groups start with `@`. Target packs start with `#`.

| Provider | Spec | Typical URL |
|---|---|---|
| Greenhouse | `greenhouse:stripe` | `job-boards.greenhouse.io/stripe` |
| Ashby | `ashby:OpenAI` | `jobs.ashbyhq.com/OpenAI` |
| Lever | `lever:includedhealth` | `jobs.lever.co/includedhealth` |

```sh
jobkit find "platform engineer" --boards greenhouse:cloudflare,ashby:Temporal
jobkit search run platform-engineering-weekly --inbox
jobkit search digest --inbox --out auto
```

One stale board prints a warning and the rest of the results still return. Pass `--strict` to fail on the first board error.

Sort with `--sort opportunity|comp|freshness`. Filter pay with `--min-comp N`. Bias rank with `--persona` values `agent-infra`, `ai-product`, `devtools`, or `backend-platform`.

After `jobkit calibrate apply` writes `~/.jobkit/calibration.yaml`, search uses those weights automatically.

## Source tags

Any source string is valid. Use these tokens so Friday stats compare lanes.

`ats` · `google-jobs` · `linkedin` · `wellfound` · `yc` · `trueup` · `hn` · `referral` · `inbound` · `community` · `stl` · `cold`

Keep any longer operator source list outside this repository. The tokens above are the public vocabulary.
