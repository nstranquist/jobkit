import { defineConfig } from 'vitepress'

// VitePress features turned on for the comparison lab:
// default theme, local MiniSearch (detailed view), lastUpdated, editLink,
// outline label, nav dropdowns, line numbers, lazy images, code groups,
// custom containers, badges, external-link icons, dark default, theme slots.
export default defineConfig({
  title: 'JobKit',
  description: 'Local-first job-search toolkit. v0.9.0.',
  lang: 'en-US',
  lastUpdated: true,
  cleanUrls: true,
  ignoreDeadLinks: true,
  appearance: 'dark',
  markdown: {
    lineNumbers: true,
    theme: { light: 'github-light', dark: 'github-dark' },
    image: { lazyLoading: true },
    container: {
      tipLabel: 'Tip',
      warningLabel: 'Warning',
      dangerLabel: 'Stop',
      infoLabel: 'Note',
      detailsLabel: 'Details',
    },
  },
  themeConfig: {
    logo: '/favicon.svg',
    siteTitle: 'JobKit',
    externalLinkIcon: true,
    outline: { level: [2, 3], label: 'On this page' },
    lastUpdated: {
      text: 'Updated',
      formatOptions: { dateStyle: 'medium', timeStyle: 'short' },
    },
    editLink: {
      pattern: 'https://github.com/nstranquist/jobkit/edit/main/docs/lab/vitepress/:path',
      text: 'Edit this page',
    },
    docFooter: {
      prev: 'Previous',
      next: 'Next',
    },
    search: {
      provider: 'local',
      options: {
        detailedView: true,
      },
    },
    nav: [
      {
        text: 'Guide',
        items: [
          { text: 'Contract', link: '/guides/contract' },
          { text: 'Install', link: '/guides/install' },
          { text: 'First hour', link: '/guides/first-hour' },
          { text: 'Weekly loop', link: '/guides/weekly-loop' },
        ],
      },
      {
        text: 'Workflow',
        items: [
          { text: 'Search', link: '/guides/search' },
          { text: 'Eligibility', link: '/guides/eligibility' },
          { text: 'Inbox', link: '/guides/inbox' },
          { text: 'Apply packages', link: '/guides/apply' },
        ],
      },
      { text: 'Reference', link: '/reference/commands' },
      { text: 'VitePress extras', link: '/vitepress-extras' },
      { text: 'Compare', link: '/compare' },
    ],
    sidebar: {
      '/': [
        {
          text: 'Start',
          collapsed: false,
          items: [
            { text: 'Contract', link: '/guides/contract' },
            { text: 'Install', link: '/guides/install' },
            { text: 'First hour', link: '/guides/first-hour' },
            { text: 'Weekly loop', link: '/guides/weekly-loop' },
          ],
        },
        {
          text: 'Workflow',
          collapsed: false,
          items: [
            { text: 'Search', link: '/guides/search' },
            { text: 'Eligibility', link: '/guides/eligibility' },
            { text: 'Inbox', link: '/guides/inbox' },
            { text: 'Apply packages', link: '/guides/apply' },
            { text: 'Tracker', link: '/guides/tracker' },
            { text: 'Coach', link: '/guides/coach' },
          ],
        },
        {
          text: 'Reference',
          collapsed: true,
          items: [
            { text: 'Command map', link: '/reference/commands' },
            { text: 'State', link: '/reference/state' },
            { text: 'Environment', link: '/reference/environment' },
            { text: 'Fix problems', link: '/reference/troubleshooting' },
          ],
        },
        {
          text: 'This lab',
          items: [
            { text: 'VitePress extras', link: '/vitepress-extras' },
            { text: 'Compare', link: '/compare' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/nstranquist/jobkit' },
    ],
    footer: {
      message: 'VitePress comparison lab · JobKit v0.9.0',
      copyright: 'MIT · local-first, no submit',
    },
  },
})
