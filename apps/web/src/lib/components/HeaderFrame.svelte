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
		/** 'page' is PageShell's max-w-5xl column; the docs pass 'docs' for their three-column max-w-7xl frame (I-396). */
		width?: 'page' | 'docs';
		/** Beside the logo: the docs put their "Docs" link here. */
		lead?: Snippet;
		/** The right-hand side of the row. */
		children: Snippet;
	} = $props();
</script>

<!-- The one header frame for the dashboard, the docs and the legal pages:
     56px tall over a --rule hairline, its content on the same max-w-5xl
     column as PageShell, so the logo sits at the same x on every page and
     does not jump when you follow a link between them. The logo carries the
     I-363 mark here as it does on the landing. Below sm the logo is the
     24px mark alone, on every page: the dashboard's five links leave no
     room for the word (mark and word left them 16px short at 390 and 46px
     short at 360), and the docs and legal headers drop it too, so the
     header looks the same wherever a phone goes (I-393). The link carries
     the name. The docs alone pass width="docs": their sidebar, text and
     "On this page" rail need max-w-7xl, so on the docs the logo sits at
     x=100 at 1440 instead of 228, on purpose (I-396). -->
<header class="border-b border-rule {sticky ? 'sticky top-0 z-30 bg-page' : ''}">
	<div
		class="mx-auto flex h-14 {width === 'docs'
			? 'max-w-7xl'
			: 'max-w-5xl'} items-center justify-between gap-3 px-5 sm:gap-6"
	>
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
