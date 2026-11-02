---
title: VitePress extras in this lab
description: Features turned on after the first lab pass.
---

# VitePress extras <Badge type="warning" text="lab only" />

This page exists so the comparison is not a default theme with three nav links. These are the VitePress surfaces that were off in the first pass.

## Chrome

| Feature | Where you see it |
|---|---|
| Dark default | Appearance toggle stays dark on first load |
| Nav dropdowns | Guide and Workflow in the top bar |
| Collapsible sidebar | Reference group starts collapsed |
| Outline label | **On this page** in the right rail |
| Theme slot | The `VitePress lab · v0.10.0` chip above the outline |
| Edit link | Footer of every Markdown page |
| lastUpdated | Footer, from git when the file is committed |
| Local search, detailed view | Search control. No Algolia |
| External link icon | Markdown links that leave the site |
| Brand gradient | Home hero name |

## Markdown

| Feature | Example on this site |
|---|---|
| Line numbers | Every fence, including this page |
| Line highlight | `find` tab on [First hour](/guides/first-hour) |
| Code groups | [Install](/guides/install) and [First hour](/guides/first-hour) |
| Containers | `info` / `tip` / `warning` / `details` on First hour |
| Badges | Title chips on this page, First hour, Install |
| Lazy images | `markdown.image.lazyLoading` |

## Intentionally off

- **Algolia** — local MiniSearch matches the local-first contract
- **Carbon ads** — not a product surface
- **i18n** — one language for this lab
- **Team page / blog** — not JobKit docs; Docusaurus already owns the changelog experiment

```sh {1,3}
echo "line numbers on"
echo "this line is not highlighted"
echo "this one is"
```
