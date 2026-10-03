<script lang="ts">
	import { pushState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { DOCS, type Doc } from '$lib/docs';

	let { doc }: { doc: Doc } = $props();

	let index = $derived(DOCS.findIndex((d) => d.slug === doc.slug));
	let prev = $derived(index > 0 ? DOCS[index - 1] : undefined);
	let next = $derived(index < DOCS.length - 1 ? DOCS[index + 1] : undefined);
	let toc = $derived(doc.headings.filter((h) => h.depth === 2));

	// The copy buttons come in doc.html, so one listener on the article
	// handles them all. The button's own word changes for the eye; the
	// live region below says the same to a screen reader, which does not
	// announce a change to the text of the button it is on.
	let article: HTMLElement | undefined = $state();
	let copyStatus = $state('');
	$effect(() => {
		if (!article) return;
		const el = article;
		const onClick = async (e: MouseEvent) => {
			const button = (e.target as Element).closest<HTMLButtonElement>('button.copy');
			if (!button) return;
			try {
				await navigator.clipboard.writeText(button.dataset.copy ?? '');
				button.textContent = 'Copied';
				copyStatus = 'Copied to the clipboard.';
			} catch {
				button.textContent = 'Copy failed';
				copyStatus = 'Could not copy to the clipboard.';
			}
			setTimeout(() => {
				button.textContent = 'Copy';
				copyStatus = '';
			}, 1500);
		};
		el.addEventListener('click', onClick);
		return () => el.removeEventListener('click', onClick);
	});

	// A code block or table wider than its box scrolls, and a scroller the
	// keyboard cannot reach is one a keyboard user cannot read to its end
	// (axe scrollable-region-focusable; only Chromium focuses one on its
	// own). Only a box that overflows gets the tab stop, checked again when
	// the column changes width, so at 1440, where the docs fit, Tab skips
	// them.
	$effect(() => {
		if (!article) return;
		const blocks = [...article.querySelectorAll<HTMLElement>('pre, .table-wrap')];
		const mark = () => {
			for (const box of blocks) {
				if (box.scrollWidth > box.clientWidth) box.tabIndex = 0;
				else box.removeAttribute('tabindex');
			}
		};
		mark();
		const ro = new ResizeObserver(mark);
		for (const box of blocks) ro.observe(box);
		return () => ro.disconnect();
	});

	// The rail marks the section you are reading: the last h2 whose top has
	// passed the reading line, a quarter of the way down the window.
	// The pass over the h2s picks the current one. It runs on scroll, at
	// most once a frame: an observer alone stays silent when one jump (a
	// wheel fling, a scrollbar drag, scrollTo) carries a heading from below
	// the reading band to above it without landing inside, and the rail
	// went stale or empty. The observers still call it for what a scroll
	// does not report: the pager arriving on a resize, a heading moving
	// when a code block above it reflows. At the page's end the last
	// section that shows is current, since a short one never reaches the line.
	let active = $state<string | undefined>();
	let rail: HTMLElement | undefined = $state();
	let pager: HTMLElement | undefined = $state();
	$effect(() => {
		if (!article || !pager || toc.length < 2) return;
		const heads = toc
			.map((h) => article!.querySelector<HTMLElement>(`h2[id="${CSS.escape(h.id)}"]`))
			.filter((el): el is HTMLElement => el !== null);
		const pick = () => {
			const line = window.innerHeight / 4;
			const atEnd =
				window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 2;
			let id: string | undefined;
			for (const el of heads) {
				const top = el.getBoundingClientRect().top;
				if (top <= line || (atEnd && top < window.innerHeight)) id = el.id;
			}
			active = id;
		};
		const io = new IntersectionObserver(pick, {
			rootMargin: '-57px 0px -75% 0px'
		});
		for (const el of heads) io.observe(el);
		const bottom = new IntersectionObserver(pick);
		if (pager) bottom.observe(pager);
		let frame = 0;
		const onScroll = () => {
			if (frame) return;
			frame = requestAnimationFrame(() => {
				frame = 0;
				pick();
			});
		};
		window.addEventListener('scroll', onScroll, { passive: true });
		pick();
		return () => {
			io.disconnect();
			bottom.disconnect();
			window.removeEventListener('scroll', onScroll);
			cancelAnimationFrame(frame);
		};
	});

	// A long rail scrolls on its own; it follows the current link.
	$effect(() => {
		if (!rail || !active) return;
		const link = rail.querySelector<HTMLElement>('[aria-current="true"]');
		if (!link) return;
		const box = rail.getBoundingClientRect();
		const at = link.getBoundingClientRect();
		if (at.top < box.top || at.bottom > box.bottom) {
			rail.scrollTop += at.top - box.top - rail.clientHeight / 3;
		}
	});

	// A section link scrolls smoothly unless the visitor asked for less
	// motion, then jumps; either way the URL takes the #section and focus
	// moves to the heading, as following the plain link would.
	function toSection(e: MouseEvent, id: string) {
		if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
		const el = document.getElementById(id);
		if (!el) return;
		e.preventDefault();
		const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		el.scrollIntoView({ behavior: still ? 'auto' : 'smooth', block: 'start' });
		el.tabIndex = -1;
		el.focus({ preventScroll: true });
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a #section on the page already open
		pushState(`#${id}`, page.state);
	}

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
     .table-wrap. From xl up the page's sections are in a 224px rail at the
     right of the docs' max-w-7xl frame; the rail's column is drawn on every
     page, empty when there is one section or none, so the text and the rail
     sit at the same x on every page (I-396). Below xl they fold under the
     description. -->
<div class="flex gap-10">
	<main id="main" class="max-w-[68ch] min-w-0 flex-1 pt-8 pb-24">
		<h1 class="text-3xl font-semibold">{doc.title}</h1>
		{#if doc.experimental}
			<p class="banner banner--warn mt-4 max-w-[33rem]" data-testid="experimental">
				Experimental. This setup is new and can change or stop working. Tell us what breaks with the
				Feedback link at the bottom of the page.
			</p>
		{/if}
		{#if doc.description}
			<p class="mt-3 max-w-[33rem] text-lg text-ink-muted">{doc.description}</p>
		{/if}

		{#if toc.length > 1}
			<!-- Below xl there is no rail, so the page's own list folds in here. -->
			<details class="group mt-6 rounded-sm border border-rule xl:hidden">
				<summary
					class="flex cursor-pointer list-none items-center justify-between px-4 py-2 text-sm font-medium [&::-webkit-details-marker]:hidden"
				>
					On this page
					<svg
						viewBox="0 0 20 20"
						class="h-4 w-4 text-ink-muted transition-transform group-open:rotate-180 motion-reduce:transition-none"
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
			class="doc prose prose-code:before:content-none prose-code:after:content-none mt-8 max-w-none [&>:is(p,ul,ol,blockquote,dl,.note)]:max-w-[33rem]"
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -- doc.html is rendered from this repo's own src/content/docs/*.md at build time, never from a user or the api -->
			{@html doc.html}
		</article>

		<!-- Always in the page, so the words that appear in it are announced. -->
		<p class="sr-only" role="status">{copyStatus}</p>

		<nav
			bind:this={pager}
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

	<!-- Sticks at 57px like the sidebar: under the header and its hairline. -->
	<div class="ml-auto hidden w-56 shrink-0 xl:block">
		{#if toc.length > 1}
			<nav
				bind:this={rail}
				class="sticky top-[57px] max-h-[calc(100dvh-57px)] overflow-y-auto overscroll-contain pt-9 pb-10"
				aria-label="On this page"
			>
				<p class="text-sm font-medium text-ink">On this page</p>
				<ul class="mt-3 border-l border-rule">
					{#each toc as h (h.id)}
						<li>
							<a
								href={`#${h.id}`}
								aria-current={active === h.id ? 'true' : undefined}
								onclick={(e) => toSection(e, h.id)}
								class="-ml-px block border-l py-1 pl-3 text-sm leading-5 {active === h.id
									? 'border-ink text-ink'
									: 'border-transparent text-ink-muted hover:text-ink'}">{h.text}</a
							>
						</li>
					{/each}
				</ul>
			</nav>
		{/if}
	</div>
</div>

<style>
	/* Forced colours draw every edge in one colour, so the current
	   section's ink edge no longer stands out; an underline marks it. */
	@media (forced-colors: active) {
		nav a[aria-current='true'] {
			text-decoration: underline;
			text-decoration-thickness: 2px;
			text-underline-offset: 0.35em;
		}
	}
</style>
