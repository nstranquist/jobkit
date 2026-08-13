# JobKit publication readiness

Status: **public v0.8.0; v0.9.0 update is locally cleared and human-gated**

Owner: `nstranquist`

Reviewed: 2026-07-12 · re-verified **2026-08-12** (`make publish-ready` + isolated `make demo`)

## Cleared locally

- Root MIT license attributes Nico-authored work to Nico Stranquist.
- Every current Git commit author is `nstranquist`; no Noise-related owner,
  domain, account, source, asset, or publication marker is present.
- Full-history `gitleaks git --redact .` passes.
- Personal bootstrap data was replaced with a synthetic profile, and tracked
  sources contain no Nico email, phone number, or resume fixture.
- `make verify` passes tests, `go vet`, and a fail-closed compiled-dependency
  license audit.
- `THIRD_PARTY_NOTICES.md` records the three compiled external modules. The audit
  pins versions and upstream license-file hashes so new or changed dependencies
  require explicit review.
- A clean copied source tree, isolated HOME, fresh dependency download, test,
  build, and license audit all pass without reading `~/.jobkit`.
- The recruiter-readable case study is maintained in `docs/case-study.md`.
- The v0.9.0 Coach source, scoring, provider, localhost, and private-state
  boundaries pass the race and publication gates.
- Strict YAML decoding, cross-process append locks, privacy telemetry schema v2,
  authenticated localhost access, scoring rubric `1.0.0`, provider resource
  limits, and the reachable-code vulnerability gate have dedicated tests.
- `apply-plan` reuses one active inbox tracker record and one deterministic
  private output directory. A regression test prevents duplicate records and
  inbox-state regression.

## Human-gated v0.9.0 update steps

1. Review `git log --oneline origin/main..main`, the README, and the case study.
2. Run `make publish-ready` and `make demo` from a clean worktree. Confirm that
   `jobkit doctor telemetry` reports only expected counts. Do not migrate a
   private telemetry log as part of a source release.
3. Restore GitHub access with `gh auth refresh -h github.com`.
4. Push the reviewed source with `git push origin main`.
5. Confirm the public run at <https://github.com/nstranquist/jobkit/actions>.
6. After CI passes, create the SemVer release:

   ```sh
   git tag -a v0.9.0 -m "JobKit v0.9.0"
   git push origin v0.9.0
   gh release create v0.9.0 \
     --repo nstranquist/jobkit \
     --verify-tag \
     --generate-notes \
     --title "JobKit v0.9.0"
   ```

7. Review the repository and release while signed out. Then record public CI
   and release evidence in the software catalog.

Do not copy `~/.jobkit`, application packages, contacts, trackers, calibration,
or real candidate profiles into the repository. Do not publish to or through a
Noise-related identity. No push, tag, release, or profile mutation is
authorized by this checklist.
