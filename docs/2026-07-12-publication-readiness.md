# JobKit publication readiness

Status: **locally cleared; publication remains human-gated**  
Owner: `nstranquist`  
Reviewed: 2026-07-12 · re-verified **2026-07-20** (`make verify-publication` + isolated `make demo`)

## Cleared locally

- Root MIT license attributes Nico-authored work to Nico Stranquist.
- Every current Git commit author is `nstranquist`; no Noise-related owner,
  domain, account, source, asset, or publication marker is present.
- Full-history `gitleaks git --redact .` passes.
- Personal bootstrap data was replaced with a synthetic profile, and tracked
  sources contain no Nico email, phone number, or resume fixture.
- `make verify` passes tests, `go vet`, and a fail-closed compiled-dependency
  license audit.
- `THIRD_PARTY_NOTICES.md` records the two compiled external modules. The audit
  pins versions and upstream license-file hashes so new or changed dependencies
  require explicit review.
- A clean copied source tree, isolated HOME, fresh dependency download, test,
  build, and license audit all pass without reading `~/.jobkit`.
- The recruiter-readable case study is maintained in `docs/case-study.md`.

## Human-gated publication steps

1. Review the final README and case study in rendered GitHub Markdown.
2. Create the public repository under `github.com/nstranquist`.
3. Confirm public CI (`.github/workflows/ci.yml`) runs
   `make verify-publication` on a supported Go toolchain.
4. Run a signed-out repository review, then update the software catalog with
   the public repository URL and green CI evidence.

Do not copy `~/.jobkit`, application packages, contacts, trackers, calibration,
or real candidate profiles into the repository. Do not publish to or through a
Noise-related identity. No public repository, push, or profile mutation is
authorized by this checklist.
