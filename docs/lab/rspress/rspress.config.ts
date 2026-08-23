import * as path from 'node:path';
import { defineConfig } from '@rspress/core';

export default defineConfig({
  root: path.join(__dirname, 'docs'),
  lang: 'en',
  title: 'JobKit',
  description: 'Local-first job-search toolkit. v0.9.0.',
  llms: true,
  themeConfig: {
    darkMode: 'dark',
    lastUpdated: true,
    enableScrollToTop: true,
    enableContentAnimation: true,
    outlineTitle: 'On this page',
    editLink: {
      docRepoBaseUrl:
        'https://github.com/nstranquist/jobkit/tree/main/docs/lab/rspress/docs',
    },
    socialLinks: [
      {
        icon: 'github',
        mode: 'link',
        content: 'https://github.com/nstranquist/jobkit',
      },
    ],
    footer: {
      message: 'Rspress comparison lab · JobKit v0.9.0 · MIT',
    },
    llmsUI: {
      injectLlmsHint: true,
      viewOptions: ['markdownLink', 'chatgpt', 'claude'],
      placement: 'outline',
    },
  },
});
