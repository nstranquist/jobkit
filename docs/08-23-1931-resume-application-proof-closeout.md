# Resume-to-application proof closeout

Date: 2026-08-23

Status: The published resume catalog and JobKit application-package bridge are verified. Five private application plans exist in `~/.jobkit/out`. Their tracker status is `interested`. JobKit has submitted no application and sent no outreach.

## Self-score

Score: 9.2/10.

The result has clean-HEAD race, test, vet, license, secret, claim, and vulnerability proof. The four producer PDFs passed strict language, parity, ATS, visual, and package gates. The five copied PDFs match their producer digests and have private `0600` permissions. Two proof gaps prevent a 10/10 score.

## Residual 1: automatic current-source proof

Owner: agent working on JobKit posting liveness.

Current control: A human must verify the official posting before package review. The 2026-08-23 dogfood run found and excluded one inbox URL that now returns HTTP 404. The five prepared plans have dated current-source notes in the private inbox ledger.

Acceptance criteria:

- `jobkit apply-plan <inbox-id>` checks the current official source before it writes a package or tracker event.
- HTTP 404, HTTP 410, an explicit expired notice, or an absent application action fails closed with a typed error.
- An unknown network result requires a human decision. It must not become `eligible` by default.
- An explicit human override records a non-empty reason in the tracker receipt.
- Tests prove that a closed posting creates no application package and no tracker mutation.

Coordination note: Existing concurrent changes own `internal/jd/liveness.go` and the related command wiring. Do not duplicate or publish another owner's working-tree changes.

## Residual 2: native Microsoft Word pagination

Owner: Nico (human-only environment gate).

Current control: PDF, Poppler, ATS, and Quick Look checks pass. Quick Look shows clean DOCX previews. Microsoft Word is not installed on this Mac, so native Word pagination is not proved.

Acceptance criteria:

- Install or use Microsoft Word.
- Open each sendable DOCX with these commands:

  ```sh
  open -a "Microsoft Word" formats-workspace/exports/sendable/Nicholas_Stranquist_Resume_v1.9.0.docx
  open -a "Microsoft Word" formats-workspace/exports/sendable/Nicholas_Stranquist_Resume_v1.9.0-fullstack.docx
  open -a "Microsoft Word" formats-workspace/exports/sendable/Nicholas_Stranquist_Resume_v1.9.1-adoption.docx
  open -a "Microsoft Word" formats-workspace/exports/sendable/Nicholas_Stranquist_Resume_v1.9.1-enterprise.docx
  ```

- Confirm one Letter page, no clipping, working links, and no substituted fonts.
- Record the Word version and screenshots in the resume visual evidence.

## Human application gate

Nico reviews each private plan, resume, letter, gap report, form packet, and source URL. Nico submits the application in the employer's browser form. Only after submission does Nico change the JobKit tracker and inbox status to `applied`.
