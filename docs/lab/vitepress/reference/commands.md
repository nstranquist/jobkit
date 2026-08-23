---
title: Command map
description: JobKit verbs by need.
---

| Need | Command |
|---|---|
| Create or inspect the profile | `jobkit init` · `profile show\|validate\|path` · `profile bootstrap --source resume.pdf` |
| Set hard constraints | `jobkit eligibility init\|check\|show\|path` |
| Saved searches | `jobkit search init\|list\|show\|run\|digest` |
| One-shot board search | `jobkit find <query> --targets NAME --inbox` |
| Target companies | `jobkit company add\|signal\|list\|show` |
| People and referrals | `jobkit contact add\|import\|referral\|touch` |
| Parse a posting | `jobkit jd parse\|fetch` |
| Score fit | `jobkit match <jd>` |
| Build materials | `jobkit resume build` · `letter build` · `prep` |
| Reviewable package | `jobkit apply-plan <jd\|url\|inbox-id>` |
| Package and track | `jobkit apply <jd>` |
| Claims allowlist | `jobkit claims init\|check\|show\|path` |
| Inbox triage | `jobkit inbox list\|slate\|stale\|outreach\|form` |
| Funnel | `jobkit track add\|set\|stats\|followups\|remind` |
| Learn ranking | `jobkit calibrate report\|apply` |
| Interview practice | `jobkit coach source\|deck\|run\|stats\|serve` |

Run `jobkit help` for the full verb list. Run `jobkit help --compact` in an agent context window.

## Agent contract

Every verb accepts `--json`. Success is `{ok, data}`. Failure is `{ok:false, error:{code, message, hint}}`.

| Exit | Meaning |
|---|---|
| `0` | Success |
| `1` | Runtime error |
| `2` | `INVALID_ARGS` |
| `3` | `NOT_FOUND` |

Flags can appear anywhere. Forms are `--k v` and `--k=v`.

An agent can prepare a package, a form-fill snippet, and an outreach draft. An agent must not submit a form or send a message without a human review.
