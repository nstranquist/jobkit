---
title: Compare with the custom HTML guide
description: How to open Starlight and the single-file HTML guide side by side.
---

This Starlight site and the custom HTML guide carry the **same JobKit v0.10.1 facts**. They differ in chrome, search, and how content is authored.

## Open both

| Surface | How to open |
|---|---|
| Custom HTML | Open [`docs/human/jobkit.html`](../../human/jobkit.html) in a browser. No build. |
| Starlight (this site) | `cd docs/starlight && pnpm dev` then http://127.0.0.1:4322 |

## What to look at

| Question | Custom HTML | Starlight |
|---|---|---|
| Authoring | One hand-written HTML file | Markdown / MDX + sidebar config |
| Search | None | Pagefind, built in |
| Navigation | In-page TOC + hash links | Persistent sidebar + breadcrumbs |
| Theme | Local dark/light toggle | Built-in theme switch |
| Ship cost | Zero dependencies | Node, Astro, Starlight |
| Fits `docs/human/` | Yes, open the file | Needs a build or `pnpm dev` |
| Agent-friendly source | Weak (HTML) | Strong (Markdown) |
| Visual identity | House playbook look | Default Starlight look |

## Keep both for now

Use the HTML file when you want a zero-build human prototype. Use Starlight when you want search, a sidebar, and Markdown as the source.

Do not treat this Starlight tree as the JobKit product runtime. JobKit remains a Go CLI. This folder is a documentation comparison lab.
