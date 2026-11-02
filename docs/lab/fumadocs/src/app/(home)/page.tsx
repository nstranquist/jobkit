import Link from 'next/link';

const cards = [
  {
    href: '/docs/guides/first-hour',
    title: 'First hour',
    body: 'Profile, constraints, discover, prepare.',
  },
  {
    href: '/docs/guides/search',
    title: 'Search',
    body: 'Greenhouse, Lever, and Ashby only.',
  },
  {
    href: '/docs/guides/apply',
    title: 'Apply packages',
    body: 'A human sends. Claims fail closed.',
  },
  {
    href: '/docs/reference/commands',
    title: 'Command map',
    body: 'Every operator verb in one table.',
  },
];

export default function HomePage() {
  return (
    <div className="flex flex-col flex-1 max-w-3xl mx-auto px-6 py-20">
      <p className="text-sm text-fd-muted-foreground mb-3">Fumadocs comparison lab · v0.10.1</p>
      <h1 className="text-5xl font-bold tracking-tight mb-4">JobKit</h1>
      <p className="text-lg text-fd-muted-foreground mb-8 max-w-2xl">
        Local-first job-search toolkit. Finds roles, scores fit, prepares
        packages, tracks the funnel. Does not submit applications. Does not
        invent experience.
      </p>
      <div className="flex flex-wrap gap-3 mb-12">
        <Link
          href="/docs"
          className="rounded-lg bg-fd-primary text-fd-primary-foreground px-4 py-2 text-sm font-medium"
        >
          Open the docs
        </Link>
        <Link
          href="/docs/guides/first-hour"
          className="rounded-lg border px-4 py-2 text-sm"
        >
          First hour
        </Link>
        <Link href="/llms.txt" className="rounded-lg border px-4 py-2 text-sm">
          llms.txt
        </Link>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {cards.map((card) => (
          <Link
            key={card.href}
            href={card.href}
            className="rounded-xl border bg-fd-card p-4 hover:border-fd-primary transition-colors"
          >
            <div className="font-medium mb-1">{card.title}</div>
            <div className="text-sm text-fd-muted-foreground">{card.body}</div>
          </Link>
        ))}
      </div>
    </div>
  );
}
