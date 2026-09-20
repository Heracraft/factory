<script lang="ts">
	import { marked } from 'marked';

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

<div class="mx-auto max-w-3xl px-5 pt-10 pb-20">
	{#if meta.status}
		<p class="banner banner--warn">Draft: {meta.status}</p>
	{/if}
	<article class="prose prose-zinc dark:prose-invert max-w-none">
		{@html html}
	</article>
</div>
