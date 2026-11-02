# JobKit documentation comparison lab

Same JobKit v0.10.1 operator facts, several viewers.

Open [`hub.html`](hub.html) after the servers below are up.

| Viewer | Command | URL | Score |
|---|---|---|---|
| Custom HTML | open `docs/human/jobkit.html` | file | 7.0 |
| Starlight | `cd ../starlight && pnpm preview --port 4322` | http://127.0.0.1:4322 | **8.6** |
| VitePress | `cd vitepress && pnpm dev` | http://127.0.0.1:4323 | **8.8** |
| Docusaurus | `cd docusaurus && pnpm start` | http://127.0.0.1:4324 | 5.8 |
| MkDocs Material | `cd mkdocs && .venv/bin/mkdocs serve -a 127.0.0.1:4325` | http://127.0.0.1:4325 | 7.4 |
| Fumadocs | `cd fumadocs && pnpm dev` | http://127.0.0.1:4326 | 8.1 |
| Vocs 1.4 | `cd vocs && ./node_modules/.bin/vocs dev --host 127.0.0.1 --port 4327` | http://127.0.0.1:4327 | 8.7 |
| Vocs 2.8 | `cd vocs2 && fnm use 22.22.1 && npx vocs dev --port 4331` | http://127.0.0.1:4331 | agent-ok; Chrome hydrate crash |
| Rspress 2 | `cd rspress && npx rspress dev --port 4328 --host 127.0.0.1` | http://127.0.0.1:4328 | 8.5 |
| Docsify | `cd docsify && python3 -m http.server 4329 --bind 127.0.0.1` | http://127.0.0.1:4329 | 5.2 |

Screenshot compare: [`shots/index.html`](shots/index.html). Scores: [`SCORECARD.md`](SCORECARD.md).

Unified catalog-docs recommendation: [`UNIFIED.md`](UNIFIED.md).

Shared Markdown source: [`content/`](content/). Each viewer adapts frontmatter and admonitions.

This lab is not the JobKit runtime. Do not publish `node_modules` or `.venv`.
