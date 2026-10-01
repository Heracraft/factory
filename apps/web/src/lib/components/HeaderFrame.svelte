<script lang="ts">
	import type { Snippet } from 'svelte';
	import Logo from './Logo.svelte';

	let {
		home,
		label,
		sticky = false,
		width = 'page',
		lead,
		children
	}: {
		/** Where the logo goes, already built with resolve(). */
		home: string;
		/** The logo link's accessible name, e.g. "repose, home". */
		label: string;
		/** The docs keep their header in view; the dashboard and legal pages scroll it away. */
		sticky?: boolean;
		/** 'page' is PageShell's max-w-5xl column; the docs pass 'docs' for their three-column max-w-7xl frame (I-396); the landing passes 'landing' for its 1120px rails, its row inset like its text (I-397). */
		width?: 'page' | 'docs' | 'landing';
		/** Beside the logo: the docs put their "Docs" link here. */
		lead?: Snippet;
		/** The right-hand side of the row. */
		children: Snippet;
	} = $props();

	// The landing's measure and inset are landing.css's --land-w and
	// --land-x, so its logo stands over the headline's first letter the way
	// every other page's stands over its title.
	const FRAME = {
		page: 'max-w-5xl px-5',
		docs: 'max-w-7xl px-5',
		landing: 'max-w-(--land-w) px-(--land-x)'
	} as const;
</script>

<!-- The one header frame for every page (the dashboard, the docs, the
     legal pages and the landing): 56px tall over a --rule hairline, its
     content by default on the same max-w-5xl column as PageShell, so the
     logo sits at the same x on every page and does not jump when you
     follow a link between them. The logo carries the I-363 mark. Below sm the logo is the
     24px mark alone, on every page: the dashboard's five links leave no
     room for the word (mark and word left them 16px short at 390 and 46px
     short at 360), and the docs and legal headers drop it too, so the
     header looks the same wherever a phone goes (I-393). The link carries
     the name. The docs alone pass width="docs": their sidebar, text and
     "On this page" rail need max-w-7xl, so on the docs the logo sits at
     x=100 at 1440 instead of 228, on purpose (I-396). The landing passes
     width="landing": its rails are 1120px apart and its text is inset by
     --land-x, so its logo sits over the headline at x=216 (I-397). -->
<header class="border-b border-rule {sticky ? 'sticky top-0 z-30 bg-page' : ''}">
	<div class="mx-auto flex h-14 {FRAME[width]} items-center justify-between gap-3 sm:gap-6">
		<div class="flex min-w-0 items-center gap-3 sm:gap-4">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass home built with resolve() -->
			<a href={home} aria-label={label} class="shrink-0"
				><span class="sm:hidden"><Logo size="sm" mark word={false} /></span><span
					class="hidden sm:inline"><Logo mark /></span
				></a
			>
			{#if lead}{@render lead()}{/if}
		</div>
		{@render children()}
	</div>
</header>
