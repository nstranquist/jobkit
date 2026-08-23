import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  guide: [
    'intro',
    {
      type: 'category',
      label: 'Start',
      items: [
        'guides/contract',
        'guides/install',
        'guides/first-hour',
        'guides/weekly-loop',
      ],
    },
    {
      type: 'category',
      label: 'Workflow',
      items: [
        'guides/search',
        'guides/eligibility',
        'guides/inbox',
        'guides/apply',
        'guides/tracker',
        'guides/coach',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/commands',
        'reference/state',
        'reference/environment',
        'reference/troubleshooting',
      ],
    },
    'compare',
  ],
};

export default sidebars;
