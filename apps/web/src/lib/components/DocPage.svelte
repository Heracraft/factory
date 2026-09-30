<script lang="ts">
	import { resolve } from '$app/paths';
	import { DOCS, type Doc } from '$lib/docs';

	let { doc }: { doc: Doc } = $props();

	let index = $derived(DOCS.findIndex((d) => d.slug === doc.slug));
	let prev = $derived(index > 0 ? DOCS[index - 1] : undefined);
	let next = $derived(index < DOCS.length - 1 ? DOCS[index + 1] : undefined);
	let toc = $derived(doc.headings.filter((h) => h.depth === 2));

	// The copy buttons come in doc.html, so one listener on the article
	// handles them all.
	let article: HTMLElement | undefined = $state();
	$effect(() => {
		if (!article) return;
		const el = article;
		const onClick = async (e: MouseEvent) => {
			const button = (e.target as Element).closest<HTMLButtonElement>('button.copy');
			if (!button) return;
			try {
				await navigator.clipboard.writeText(button.dataset.copy ?? '');
				button.textContent = 'Copied';
			} catch {
				button.textContent = 'Copy failed';
			}
			setTimeout(() => (button.textContent = 'Copy'), 1500);
		};
		el.addEventListener('click', onClick);
		return () => el.removeEventListener('click', onClick);
	});

	function href(slug: string): string {
		return slug === 'index' ? resolve('/docs') : resolve('/docs/[slug]', { slug });
	}
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- every internal href here comes from href(), which builds it with resolve() -->

<svelte:head>
	<title>{doc.slug === 'index' ? 'repose docs' : `${doc.title} · repose docs`}</title>
	<meta name="description" content={doc.description} />
</svelte:head>

<!-- Two measures. Running text (paragraphs, lists, quotes, notes) stops
     at 33rem, about 68 characters of the body sans a line; with max-w-none
     it ran 94 to 114 at 1440. The column itself is 68ch (622px), wide enough
     for 70 columns of the 13px mono with no sideways scroll, which is what
     the docs are written to fit (I-345, docs.test.ts); code blocks, tables
     and the h2 rules use all of it, and a wider table scrolls inside
     .table-wrap. The page's own headings are listed under its link in the
     sidebar from lg up, and in the fold below the description under lg:
     a right rail beside the sidebar, in the header's max-w-5xl column,
     would squeeze the code under 70 columns. -->
<main class="max-w-[68ch] min-w-0 pt-8 pb-24">
	<h1 class="text-3xl font-semibold sm:text-4xl">{doc.title}</h1>
	{#if doc.description}
		<p class="mt-3 max-w-[33rem] text-lg text-ink-muted">{doc.description}</p>
	{/if}

	{#if toc.length > 1}
		<!-- Below lg the sidebar is a closed drawer, so the page's own list folds in here. -->
		<details class="group mt-6 rounded-sm border border-rule lg:hidden">
			<summary
				class="flex cursor-pointer list-none items-center justify-between px-4 py-2 text-sm font-medium [&::-webkit-details-marker]:hidden"
			>
				On this page
				<svg
					viewBox="0 0 20 20"
					class="h-4 w-4 text-ink-muted transition-transform group-open:rotate-180"
					aria-hidden="true"
					fill="none"><path d="M5 8l5 5 5-5" stroke="currentColor" stroke-width="1.5" /></svg
				>
			</summary>
			<ul class="space-y-1.5 border-t border-rule px-4 py-3 text-sm">
				{#each toc as h (h.id)}
					<li>
						<a href={`#${h.id}`} class="text-ink-muted hover:text-ink">{h.text}</a>
					</li>
				{/each}
			</ul>
		</details>
	{/if}

	<article
		bind:this={article}
		class="doc prose prose-zinc dark:prose-invert prose-code:before:content-none prose-code:after:content-none mt-8 max-w-none [&>:is(p,ul,ol,blockquote,dl,.note)]:max-w-[33rem]"
	>
		<!-- eslint-disable-next-line svelte/no-at-html-tags -- doc.html is rendered from this repo's own src/content/docs/*.md at build time, never from a user or the api -->
		{@html doc.html}
	</article>

	<nav
		class="mt-16 grid gap-4 border-t border-rule pt-6 sm:grid-cols-2"
		aria-label="Previous and next page"
	>
		{#if prev}
			<a
				href={href(prev.slug)}
				class="group rounded-sm border border-rule px-4 py-3 hover:border-rule-strong"
			>
				<span class="block text-xs text-ink-muted">Previous</span>
				<span class="mt-0.5 block font-medium group-hover:underline">{prev.title}</span>
			</a>
		{:else}
			<span></span>
		{/if}
		{#if next}
			<a
				href={href(next.slug)}
				class="group rounded-sm border border-rule px-4 py-3 text-right hover:border-rule-strong"
			>
				<span class="block text-xs text-ink-muted">Next</span>
				<span class="mt-0.5 block font-medium group-hover:underline">{next.title}</span>
			</a>
		{/if}
	</nav>
</main>
