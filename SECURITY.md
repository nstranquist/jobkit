# Security Policy

## Scope

JobKit is a **local-first** CLI. By default it does not send profile data to a
JobKit-operated service. Network calls are limited to:

- Public job-board APIs you explicitly query (`find`, `search run`, `jd fetch`)
- Optional headless Chrome for PDF export (`JOBKIT_CHROME_BIN`)

## Data handling

| Data | Location | Publication |
| --- | --- | --- |
| Master profile, searches, inbox, tracker | `JOBKIT_HOME` (default `~/.jobkit`) | **Never** ship in source |
| Claims allowlist | `~/.jobkit/claims.yaml` | Private |
| Repo examples | `examples/` | Synthetic only |

## Reporting

If you find a vulnerability in JobKit source (e.g. path traversal writing
outside `JOBKIT_HOME`, secret leakage in fixtures, unsafe PDF/Chrome flags),
email the maintainer through the GitHub profile contact or open a
[private security advisory](https://github.com/nstranquist/jobkit/security/advisories/new).

## Secrets in this tree

- Run `gitleaks git` / `gitleaks dir` via `make verify-publication` before any
  publish.
- Do not put real API keys, resume PDFs, or LinkedIn exports into the repo.
