<script lang="ts">
	import { resolve } from '$app/paths';
	import { DOCS, type Doc } from '$lib/docs';

	let { doc }: { doc: Doc } = $props();

	let index = $derived(DOCS.findIndex((d) => d.slug === doc.slug));
	let prev = $derived(index > 0 ? DOCS[index - 1] : undefined);
	let next = $derived(index < DOCS.length - 1 ? DOCS[index + 1] : undefined);
	let toc = $derived(doc.headings.filter((h) => h.depth === 2));

	function href(slug: string): string {
		return slug === 'index' ? resolve('/docs') : resolve('/docs/[slug]', { slug });
	}
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- every internal href here comes from href(), which builds it with resolve() -->

<svelte:head>
	<title>{doc.slug === 'index' ? 'repose docs' : `${doc.title} · repose docs`}</title>
	<meta name="description" content={doc.description} />
</svelte:head>

<div class="flex gap-10">
	<main class="min-w-0 flex-1 pt-8 pb-24">
		<h1 class="text-3xl font-semibold sm:text-4xl">{doc.title}</h1>
		{#if doc.description}
			<p class="mt-3 text-lg text-zinc-600 dark:text-zinc-400">{doc.description}</p>
		{/if}

		<article
			class="doc prose prose-zinc dark:prose-invert prose-code:before:content-none prose-code:after:content-none mt-8 max-w-none"
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -- doc.html is rendered from this repo's own src/content/docs/*.md at build time, never from a user or the api -->
			{@html doc.html}
		</article>

		<nav
			class="mt-16 grid gap-4 border-t border-[var(--rule)] pt-6 sm:grid-cols-2"
			aria-label="Previous and next page"
		>
			{#if prev}
				<a
					href={href(prev.slug)}
					class="group rounded-sm border border-[var(--rule)] px-4 py-3 hover:border-[var(--rule-strong)]"
				>
					<span class="block text-xs text-zinc-600 dark:text-zinc-400">Previous</span>
					<span class="mt-0.5 block font-medium group-hover:underline">{prev.title}</span>
				</a>
			{:else}
				<span></span>
			{/if}
			{#if next}
				<a
					href={href(next.slug)}
					class="group rounded-sm border border-[var(--rule)] px-4 py-3 text-right hover:border-[var(--rule-strong)]"
				>
					<span class="block text-xs text-zinc-600 dark:text-zinc-400">Next</span>
					<span class="mt-0.5 block font-medium group-hover:underline">{next.title}</span>
				</a>
			{/if}
		</nav>
	</main>

	{#if toc.length > 1}
		<aside class="hidden w-52 shrink-0 xl:block">
			<nav class="sticky top-14 pt-9 pb-10" aria-label="On this page">
				<p class="text-sm font-medium">On this page</p>
				<ul class="mt-2 space-y-1.5 text-sm">
					{#each toc as h (h.id)}
						<li>
							<a
								href={`#${h.id}`}
								class="text-zinc-700 hover:text-zinc-950 dark:text-zinc-300 dark:hover:text-zinc-50"
								>{h.text}</a
							>
						</li>
					{/each}
				</ul>
			</nav>
		</aside>
	{/if}
</div>
