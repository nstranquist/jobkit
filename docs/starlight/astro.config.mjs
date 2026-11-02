// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

export default defineConfig({
	site: 'http://127.0.0.1:4322',
	server: { port: 4322, host: '127.0.0.1' },
	integrations: [
		starlight({
			title: 'JobKit',
			description: 'Local-first job-search toolkit. v0.10.0.',
			tagline: 'Finds roles. Scores fit. Prepares packages. Does not submit.',
			lastUpdated: true,
			pagination: true,
			editLink: {
				baseUrl: 'https://github.com/nstranquist/jobkit/edit/main/docs/starlight/',
			},
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/nstranquist/jobkit',
				},
			],
			customCss: ['./src/styles/theme.css'],
			expressiveCode: {
				themes: ['github-dark', 'github-light'],
				styleOverrides: {
					borderRadius: '10px',
				},
			},
			plugins: [
				starlightLlmsTxt({
					projectName: 'JobKit',
					description:
						'Local-first job-search toolkit. Finds roles, scores fit, prepares packages. Does not submit applications.',
				}),
			],
			sidebar: [
				{
					label: 'Start',
					collapsed: false,
					items: [
						{ label: 'Compare with custom HTML', slug: 'compare' },
						{ label: 'Contract', slug: 'guides/contract' },
						{ label: 'Install', slug: 'guides/install' },
						{ label: 'First hour', slug: 'guides/first-hour' },
						{ label: 'Weekly loop', slug: 'guides/weekly-loop' },
					],
				},
				{
					label: 'Workflow',
					collapsed: false,
					items: [
						{ label: 'Search', slug: 'guides/search' },
						{ label: 'Eligibility', slug: 'guides/eligibility' },
						{ label: 'Inbox', slug: 'guides/inbox' },
						{ label: 'Apply packages', slug: 'guides/apply' },
						{ label: 'Tracker', slug: 'guides/tracker' },
						{ label: 'Coach', slug: 'guides/coach' },
					],
				},
				{
					label: 'Reference',
					collapsed: true,
					items: [{ autogenerate: { directory: 'reference' } }],
				},
			],
		}),
	],
});
