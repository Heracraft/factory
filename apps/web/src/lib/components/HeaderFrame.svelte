<script lang="ts">
	import type { Snippet } from 'svelte';
	import Logo from './Logo.svelte';

	let {
		home,
		label,
		sticky = false,
		compact = false,
		lead,
		children
	}: {
		/** Where the logo goes, already built with resolve(). */
		home: string;
		/** The logo link's accessible name, e.g. "repose, home". */
		label: string;
		/** The docs keep their header in view; the dashboard and legal pages scroll it away. */
		sticky?: boolean;
		/** Below sm the logo is the mark alone, for a row whose links need the room. */
		compact?: boolean;
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
     I-363 mark here as it does on the landing. Below sm the wordmark drops
     to its 24px cut; a compact row (the dashboard's, with five links) shows
     the 28px mark alone there, since mark and word left the links 16px
     short at 390 and 46px short at 360. -->
<header class="border-b border-[var(--rule)] {sticky ? 'sticky top-0 z-30 bg-[var(--page)]' : ''}">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between gap-3 px-5 sm:gap-6">
		<div class="flex min-w-0 items-center gap-3 sm:gap-4">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- callers pass home built with resolve() -->
			<a href={home} aria-label={label} class="shrink-0"
				><span class="sm:hidden"
					>{#if compact}<Logo mark word={false} />{:else}<Logo size="sm" mark />{/if}</span
				><span class="hidden sm:inline"><Logo mark /></span></a
			>
			{#if lead}{@render lead()}{/if}
		</div>
		{@render children()}
	</div>
</header>
