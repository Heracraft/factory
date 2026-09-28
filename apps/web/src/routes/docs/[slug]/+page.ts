import { DOCS, MOVED } from '$lib/docs';
import type { EntryGenerator } from './$types';

// Every page, and every old slug that now redirects, is prerendered. Any
// other slug is rendered on request and says there's no such page.
export const prerender = 'auto';

export const entries: EntryGenerator = () =>
	[...DOCS.filter((d) => d.slug !== 'index').map((d) => d.slug), ...Object.keys(MOVED)].map(
		(slug) => ({ slug })
	);
