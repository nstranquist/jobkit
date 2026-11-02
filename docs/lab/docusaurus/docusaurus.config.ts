import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// Docusaurus strengths used here: classic preset, versioning-ready docs,
// blog changelog, Mermaid, local search, i18n hook, color mode.
const config: Config = {
  title: 'JobKit',
  tagline: 'Local-first job-search toolkit. Does not submit applications.',
  favicon: 'img/favicon.ico',
  future: {v4: true},
  url: 'http://127.0.0.1:4324',
  baseUrl: '/',
  organizationName: 'nstranquist',
  projectName: 'jobkit',
  onBrokenLinks: 'warn',
  markdown: {mermaid: true},
  themes: [
    '@docusaurus/theme-mermaid',
    [
      require.resolve('@easyops-cn/docusaurus-search-local'),
      {
        hashed: true,
        language: ['en'],
        indexBlog: true,
        indexDocs: true,
        docsRouteBasePath: '/docs',
      },
    ],
  ],
  i18n: {defaultLocale: 'en', locales: ['en']},
  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: 'docs',
        },
        blog: {
          showReadingTime: true,
          blogTitle: 'JobKit changelog',
          blogDescription: 'Release notes for the JobKit CLI.',
          postsPerPage: 5,
        },
        theme: {customCss: './src/css/custom.css'},
      } satisfies Preset.Options,
    ],
  ],
  themeConfig: {
    colorMode: {respectPrefersColorScheme: true},
    navbar: {
      title: 'JobKit',
      items: [
        {type: 'docSidebar', sidebarId: 'guide', position: 'left', label: 'Guide'},
        {to: '/blog', label: 'Changelog', position: 'left'},
        {href: 'https://github.com/nstranquist/jobkit', label: 'GitHub', position: 'right'},
      ],
    },
    footer: {
      style: 'dark',
      copyright: `Docusaurus comparison lab · JobKit v0.10.0 · MIT`,
    },
    prism: {theme: prismThemes.github, darkTheme: prismThemes.dracula},
  } satisfies Preset.ThemeConfig,
};

export default config;
