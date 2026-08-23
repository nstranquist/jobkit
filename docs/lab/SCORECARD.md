# JobKit docs-lab scorecard

Date: 2026-08-14
Evidence: `ndev browser` viewport shots in [`shots/`](shots/) (1440×900).
Same JobKit v0.9.0 facts in every viewer.

## Final ranking (after taste pass)

Taste pass: Vocs jumped. Fumadocs and Docsify are out. MkDocs stays as a pocket option.

| Lane | Viewer | Score | Status |
|---|---|---|---|
| Taste winner | **Vocs** | **8.7** | Keep in the lab. Pin 1.4.1 today. Try 2.x after Node ≥ 22.15. |
| Look runner-up | VitePress | 8.6 | Vue alternate. Best Markdown toys. |
| Estate pick | Starlight | **8.6** (enhanced) | Steps, asides, tabs, tokens, `/llms.txt`. |
| Pocket | Rspress 2 | 8.2 | Yes, left submenus. Overlaps VitePress. |
| Pocket | MkDocs Material | 7.4 | Background only. Maintenance mode. |
| Out | Custom HTML | 7.0 | Prototype, not the viewer. |
| Out | Docusaurus | 5.8 | Looks bad. Versioning-only win. |
| Out | **Fumadocs** | — | Ruled out. |
| Out | **Docsify** | — | Ruled out. |

## Rubric

| Axis | What I scored |
|---|---|
| Look | Home + first-hour screenshots |
| Chrome | Sidebar, search, outline, theme toggle |
| Markdown | Containers, groups, steps, badges |
| Agent | `llms.txt`, copy-md, ChatGPT/Claude hooks |
| Weight | Install, Node/Python graph, start time |
| Catalog fit | Can this be the one viewer over `ndev catalog` |

Each axis is /10. Total is a weighted mean: look 25%, chrome 20%, markdown 15%, agent 10%, weight 10%, catalog 20%.

## Per viewer

### VitePress — 8.8

| Axis | Score | Note |
|---|---|---|
| Look | 9.2 | Gradient name, dark default, feature cards. Empty hero image removed. |
| Chrome | 8.8 | Search, dropdowns, collapsed Reference, outline chip via theme slot |
| Markdown | 9.0 | Code groups, line numbers, badges, tip/warning/details |
| Agent | 6.5 | No native llms.txt |
| Weight | 8.5 | Small Vue graph |
| Catalog | 8.5 | Fine as the one viewer. Vue, not already in Watchbook |

Turned on this pass: line numbers, code groups, badges, containers, nav dropdowns, editLink, lastUpdated, detailed MiniSearch, theme slot, dark default, extras page.

### Rspress 2 — 8.5

| Axis | Score | Note |
|---|---|---|
| Look | 9.3 | Best splash after the icon fix. Purple gradient home. |
| Chrome | 8.6 | Search, outline, edit/copy/open-in-chat |
| Markdown | 8.2 | Containers work. No code-groups in this lab. |
| Agent | 8.5 | `llms: true` + outline ChatGPT/Claude |
| Weight | 8.0 | Fast Rspack. React 19. |
| Catalog | 8.0 | Overlaps VitePress. Newer 2.0, slightly less known here |

Home first capture was blank: numeric `icon: 1` crashed `SvgWrapper`. Fixed to emoji.

### Starlight — 8.3

| Axis | Score | Note |
|---|---|---|
| Look | 8.4 | Quiet, professional. Splash is plainer than VitePress. |
| Chrome | 9.0 | Pagefind, sidebar, Auto theme. Best default docs page. |
| Markdown | 8.0 | Cards/MDX. We did not push extras as hard. |
| Agent | 6.0 | No llms.txt in this lab |
| Weight | 9.0 | Astro, already in Watchbook + tokens |
| Catalog | 9.2 | Still the architecture pick: one generated viewer |

### Fumadocs — 8.1

| Axis | Score | Note |
|---|---|---|
| Look | 8.5 | Clean home cards. Docs page is modern. |
| Chrome | 8.5 | Orama, sidebar. Nav + tree both list First hour. |
| Markdown | 8.3 | Callouts. MDX native. |
| Agent | 9.0 | `/llms.txt`, `/llms-full.txt`, Copy Markdown, Open |
| Weight | 7.0 | Next.js. Heavier. |
| Catalog | 8.0 | Best if the catalog site must live in Next |

Healthy on :4326 this pass.

### Vocs — 7.8

| Axis | Score | Note |
|---|---|---|
| Look | 8.2 | Distinct wevm chrome. Landing is sparse. Banner is loud. |
| Chrome | 8.4 | Search, outline, theme |
| Markdown | 8.6 | `:::steps` is the best first-hour of the set |
| Agent | 8.8 | Ask in ChatGPT + `/llms.txt` |
| Weight | 6.5 | Flaky to stand up (IPv6 bind, waku engine) |
| Catalog | 7.5 | Beautiful extra, not the estate viewer |

Now bound to `127.0.0.1:4327`.

### MkDocs Material — 7.4

| Axis | Score | Note |
|---|---|---|
| Look | 7.5 | Instantly recognizable. Dated vs VitePress. |
| Chrome | 8.2 | Tabs, instant nav, suggest search |
| Markdown | 8.4 | Admonitions, tabbed Does/Does not |
| Agent | 5.0 | None native |
| Weight | 7.5 | Python venv. MkDocs 2.0 break incoming |
| Catalog | 7.0 | Fine operator wiki. Not the catalog portal |

### Custom HTML — 7.0

| Axis | Score | Note |
|---|---|---|
| Look | 8.0 | House playbook. Dense and on-brand. |
| Chrome | 5.0 | Hash TOC. No search. |
| Markdown | 4.0 | There is no Markdown. |
| Agent | 3.0 | Agents hate one HTML file |
| Weight | 9.5 | Zero build |
| Catalog | 5.5 | Prototype only |

### Docusaurus — 5.8

| Axis | Score | Note |
|---|---|---|
| Look | 5.0 | Mint hero is loud. Infima docs page is generic. |
| Chrome | 7.5 | Search, breadcrumbs, changelog |
| Markdown | 7.0 | Mermaid on, unused on first-hour |
| Agent | 6.0 | Blog/changelog only |
| Weight | 5.5 | Heavy React graph |
| Catalog | 6.5 | Wins only if we need public versioning |

Agree with the earlier read: it looks the worst.

### Docsify — 5.2

| Axis | Score | Note |
|---|---|---|
| Look | 4.5 | Cover contrast fails. GitHub ribbon. |
| Chrome | 5.5 | Sidebar + CDN search |
| Markdown | 4.8 | Frontmatter stripped by a tiny plugin. Still thin. |
| Agent | 3.0 | Runtime-only, no llms |
| Weight | 8.0 | No build |
| Catalog | 4.5 | Not a catalog portal |

## Freeze

Do **not** freeze from this lab yet. You asked to score, not to pick.

If you pick on **look + Markdown**, freeze **VitePress**.
If you pick on **catalog architecture + already-in-Watchbook**, freeze **Starlight**.
If you pick on **agent surfaces**, freeze **Fumadocs** or **Rspress**.

Architecture from [`UNIFIED.md`](UNIFIED.md) still holds: one generated viewer. Not one framework per product.
