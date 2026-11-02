# Unified documentation for catalog items

Status: recommendation, 2026-08-14
Question: one docs site for every catalog product, recipe, and operator surface.

## Recommendation

Build **one Starlight site** that *renders* the catalog. Do not pick a new
framework per product. Do not replace the catalog, Catalog Atlas, or
docs-puller.

| Layer | Job | Tool |
|---|---|---|
| Inventory | What exists, status, refs | `ndev catalog` |
| Operator visualization | Graph, drift, valuation | Catalog Atlas |
| Vendor / upstream docs | Search third-party manuals | docs-puller |
| **Human product docs** | How to use JobKit, ndev, nvault, … | **One Starlight site** |
| Knowledge | Findings, KEPs, ADRs | Keep as Markdown sources |

Watchbook already ships Astro 7 + Starlight + `@nicos/design-tokens`. Hugo is
the scored Go *publishing pattern* (`refs/hugo`, lift clone), not the reader UI.

## Why Starlight for the viewer

- Purpose-built docs chrome (sidebar, search, TOC, dark mode).
- Markdown/MDX is agent-safe. The custom HTML file is not.
- Zero JS by default. Fast enough to generate from catalog.
- Already in the portfolio (Watchbook).
- Comparison labs below can overturn this if Fumadocs or VitePress wins on
  look after you sit with them. The *architecture* stays the same: one viewer,
  catalog as source.

2026-08-14 after enhance pass: Starlight is now **8.6** (steps, asides, tabs,
tokens, `/llms.txt`). Vocs **1.4.1** on :4327 is still the taste reference.
Vocs **2.8.5** installs on Node 22.22.1 (`:4331`) but Chrome hydration
crashes (`ScrollRestoration`). Do not freeze on 2.x yet.

## What “catalog items” should appear

Include:

- `product.*` with `launch: built|launched` and a public or operator story
- `recipe.*` champion docs
- Operator golden paths (`ndev`, JobKit, docs-puller, scratchpad)
- A short KEP index, not every KEP body

Exclude:

- Every `human-doc.*` prototype (keep `docs/human/` as the lab)
- Every command leaf (`ndev.foo.bar`)
- Private `~/.jobkit` and valuation internals
- Vendor corpora (those stay in docs-puller)

## How it should be generated

1. Author Markdown next to the product (`docs/` in JobKit, nvault, …).
2. `documented_by: human-doc.<slug>` or `doc.<slug>` stays the catalog edge.
3. A generator (`ndev catalog docs emit`, shipped 2026-08-16) writes:
   - sidebar from catalog surfaces
   - one page per included product (description + entrypoint + checks)
   - deep pages only when the product tree has real Markdown
4. Starlight builds that tree. Garrid tokens can theme it the way Watchbook
   already does.

Do **not** hand-maintain 200 HTML files. The custom JobKit HTML was a
prototype. Starlight already beat it on navigation and search.

## Comparison lab

Same JobKit v0.10.0 facts, six viewers:

| Viewer | Port | Distinctive features we turned on |
|---|---|---|
| Custom HTML | file | Zero build, house playbook look |
| Starlight | 4322 | Pagefind, sidebar, splash |
| VitePress | 4323 | Code groups, badges, theme slot, editLink, detailed MiniSearch |
| Docusaurus | 4324 | Mermaid, changelog blog, local search |
| MkDocs Material | 4325 | Instant nav, tabs, admonitions, mermaid |
| Fumadocs | 4326 | Orama, Copy Markdown, `/llms.txt` |
| Vocs | 4327 | Steps, Ask-in-ChatGPT, banner, IPv4 bind |
| Rspress 2 | 4328 | Gradient home, llms UI, edit/copy/open |
| Docsify | 4329 | Runtime Markdown, coverpage, CDN search |

Open `docs/lab/hub.html` after the servers start.

## Missed frameworks we considered and did not fully stand up

| Framework | Why it is on the list | Why it is not a lab yet |
|---|---|---|
| **Rspress** | Very fast Rspack docs (ByteDance) | **Now in the lab on :4328.** Overlaps VitePress. |
| **mdBook** | Rust-book simplicity | Weak search and theming for a catalog portal |
| **Zensical** | Material-for-MkDocs successor | Too new to bet the estate on |
| **Quartz** | Digital-garden / wiki | Wrong metaphor for product docs |
| **Sphinx** | Python API docs | Wrong language and weight |
| **Antora** | Multi-repo versioning | Overkill until many public versioned products |
| **Mintlify** | Beautiful hosted docs | Not fully OSS. Conflicts with local-first |
| **Hugo** | Already creffed. Strong generator | Theme quality is DIY. Use as emit engine later if Go-only emit matters |

## Decision to make after you look

Sit with Starlight, VitePress, and Fumadocs for ten minutes each. Then freeze
**one** viewer. After that, write the emit command and stop adding frameworks.
