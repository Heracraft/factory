import { error } from '@sveltejs/kit';
import { DOCS, MOVED, docBySlug, movedDoc } from '$lib/docs';
import type { EntryGenerator, PageLoad } from './$types';

// Every page, and every old slug that now redirects, is prerendered. Any
// other slug is rendered on request and says there's no such page.
export const prerender = 'auto';

export const entries: EntryGenerator = () =>
	[...DOCS.filter((d) => d.slug !== 'index').map((d) => d.slug), ...Object.keys(MOVED)].map(
		(slug) => ({ slug })
	);

// A slug that is no page and no moved page is a 404, so a stale link reads
// as missing to people and to monitors (it answered 200, or 500 in the
// image). docs/+error.svelte shows "No such page".
export const load: PageLoad = ({ params }) => {
	if (!docBySlug(params.slug) && !movedDoc(params.slug)) error(404, 'No such page');
};
