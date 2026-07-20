# JobKit: an evidence-preserving job-search operating system

## Problem

Serious job searches fragment quickly across job boards, resume variants,
contacts, outreach drafts, application packages, and follow-up notes. Generic
automation often optimizes for application volume while losing evidence,
provenance, and human review.

## Nico's role

Nico designed and built JobKit as a local-first Go CLI: the command model,
profile and tracker schemas, board adapters, fit and opportunity scoring,
calibration loop, document generation, agent contract, and verification suite.

## Key decisions

- Keep candidate state local and explicit: YAML for editable profiles and
  searches, append-only JSONL for application and relationship history.
- Make tailoring deterministic. Resume, cover-letter, and interview-prep output
  may reorder or select verified evidence but cannot invent experience.
- Separate discovery from submission. JobKit prepares reviewable application
  plans and form-fill packets; it does not auto-submit applications.
- Rank opportunities using compensation, freshness, saturation, persona fit,
  and observed outcomes instead of keyword coverage alone.
- Learn from the funnel through explicit calibration weights while preserving a
  stable default when outcome data is still sparse.
- Expose a typed JSON envelope and stable exit codes so agents can use the same
  surface without bypassing human review.

## Verified result

JobKit provides one coherent workflow from public-board discovery and hidden-
market company signals through inbox triage, contact/referral tracking, JD
analysis, tailored materials, interview preparation, application tracking,
follow-ups, and outcome calibration. The full Go test suite, vet, clean-copy
build, full-history secret scan, and dependency-license audit pass locally.

## Evidence boundary

The source is prepared for a future MIT-licensed repository owned by
`nstranquist`, but no public repository or CI is claimed yet. Real candidate
profiles, applications, contacts, trackers, and generated packages remain
private under `~/.jobkit` and are not publication inputs.
