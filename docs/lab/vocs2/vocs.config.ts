import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'JobKit',
  description: 'Local-first job-search toolkit. v0.10.0.',
  checkDeadlinks: 'warn',
  editLink: {
    pattern:
      'https://github.com/nstranquist/jobkit/edit/main/docs/lab/vocs2/src/pages/:path',
    text: 'Edit this page',
  },
  socials: [
    {
      icon: 'github',
      link: 'https://github.com/nstranquist/jobkit',
    },
  ],
  theme: {
    accentColor: {
      light: '#1f6feb',
      dark: '#79c0ff',
    },
  },
  llms: {
    generateMarkdown: true,
  },
  sidebar: [
    {
      text: 'Start',
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
        { text: 'Tracker', link: '/guides/tracker' },
        { text: 'Coach', link: '/guides/coach' },
      ],
    },
    {
      text: 'Reference',
      items: [
        { text: 'Command map', link: '/reference/commands' },
        { text: 'State', link: '/reference/state' },
        { text: 'Environment', link: '/reference/environment' },
        { text: 'Fix problems', link: '/reference/troubleshooting' },
      ],
    },
  ],
  topNav: [
    { text: 'Guide', link: '/guides/first-hour' },
    { text: 'Reference', link: '/reference/commands' },
    { text: 'Compare', link: '/compare' },
  ],
  vite: {
    server: {
      host: '127.0.0.1',
      port: 4331,
      strictPort: true,
    },
  },
})
