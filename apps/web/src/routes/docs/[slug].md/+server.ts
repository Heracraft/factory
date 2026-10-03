import { error } from '@sveltejs/kit';
import { docBySlug } from '$lib/docs';
import { docMarkdown, SLUGS } from '$lib/llms';
import type { EntryGenerator, RequestHandler } from './$types';

export const prerender = true;

export const entries: EntryGenerator = () => SLUGS.map((slug) => ({ slug }));

export const GET: RequestHandler = ({ params }) => {
	const doc = docBySlug(params.slug);
	if (!doc) error(404, 'No such page');
	return new Response(docMarkdown(doc), {
		headers: { 'content-type': 'text/markdown; charset=utf-8' }
	});
};
