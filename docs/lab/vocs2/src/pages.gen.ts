// deno-fmt-ignore-file
// biome-ignore format: generated types do not need formatting
// prettier-ignore
import type { PathsForPages } from 'waku/router'

// prettier-ignore
type Page =
  | { path: '/compare'; render: 'static' }
  | { path: '/guides/apply'; render: 'static' }
  | { path: '/guides/coach'; render: 'static' }
  | { path: '/guides/contract'; render: 'static' }
  | { path: '/guides/eligibility'; render: 'static' }
  | { path: '/guides/first-hour'; render: 'static' }
  | { path: '/guides/inbox'; render: 'static' }
  | { path: '/guides/install'; render: 'static' }
  | { path: '/guides/search'; render: 'static' }
  | { path: '/guides/tracker'; render: 'static' }
  | { path: '/guides/weekly-loop'; render: 'static' }
  | { path: '/'; render: 'static' }
  | { path: '/reference/commands'; render: 'static' }
  | { path: '/reference/environment'; render: 'static' }
  | { path: '/reference/state'; render: 'static' }
  | { path: '/reference/troubleshooting'; render: 'static' }

// prettier-ignore
declare module 'waku/router' {
  interface RouteConfig {
    paths: PathsForPages<Page>
  }
  interface CreatePagesConfig {
    pages: Page
  }
}
