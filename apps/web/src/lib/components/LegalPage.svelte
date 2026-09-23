<script lang="ts">
	import { marked } from 'marked';
	import { resolve } from '$app/paths';
	import Logo from './Logo.svelte';

	let { raw }: { raw: string } = $props();

	// content/legal/*.md files carry a small YAML-ish frontmatter block
	// (title, effective, status) that 14-security's docs use to mark drafts;
	// this is the only place that reads it, so a tiny hand-rolled parser
	// beats pulling in a YAML dependency for three known keys.
	function parseFrontmatter(text: string): { meta: Record<string, string>; body: string } {
		const match = /^---\n([\s\S]*?)\n---\n([\s\S]*)$/.exec(text);
		if (!match) return { meta: {}, body: text };
		const meta: Record<string, string> = {};
		for (const line of match[1].split('\n')) {
			const i = line.indexOf(':');
			if (i === -1) continue;
			meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
		}
		return { meta, body: match[2] };
	}

	let { meta, body } = $derived(parseFrontmatter(raw));
	let html = $derived(marked.parse(body) as string);
</script>

<header class="mx-auto flex max-w-3xl items-center justify-between px-5 py-6">
	<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
	<nav class="flex gap-4 text-sm text-zinc-500 dark:text-zinc-400">
		<a href={resolve('/terms')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Terms</a>
		<a href={resolve('/privacy')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Privacy</a>
	</nav>
</header>

<main class="mx-auto max-w-3xl px-5 pt-6 pb-24">
	{#if meta.status}
		<p class="banner banner--warn">Draft: {meta.status}</p>
	{/if}
	<article
		class="prose prose-zinc dark:prose-invert prose-headings:font-display prose-a:text-blue-700 dark:prose-a:text-blue-300 max-w-none"
	>
		<!-- eslint-disable-next-line svelte/no-at-html-tags -- `raw` only ever comes from this repo's own src/content/legal/*.md via a ?raw import, never from a user or the api -->
		{@html html}
	</article>
</main>
