<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Crumb {
		label: string;
		href?: string;
	}

	let {
		title,
		crumbs = [],
		width = 'list',
		lede,
		action,
		children
	}: {
		title: string;
		crumbs?: Crumb[];
		width?: 'form' | 'list';
		lede?: string;
		action?: Snippet;
		children: Snippet;
	} = $props();
</script>

<!-- A <main> landmark rather than a <div>: it was the one audit every
     signed-in page failed (Lighthouse landmark-one-main, 98/100), and it is
     what a screen reader's "skip to main content" jumps to. The main always
     spans the header's max-w-5xl column; a form page narrows only its
     content to max-w-2xl, flush left, so its title starts under the logo
     instead of 176px to the right of it at 1440. -->
<main class="mx-auto max-w-5xl px-5 pt-10 pb-24">
	<div class={width === 'form' ? 'max-w-2xl' : undefined}>
		{#if crumbs.length}
			<!-- The separator is a middle dot, as DESIGN-LANGUAGE.md "Page frame" says. -->
			<p class="mb-3 text-sm text-ink-muted">
				{#each crumbs as crumb, i (crumb.label)}
					{#if i > 0}<span class="mx-1.5" aria-hidden="true">·</span>{/if}
					{#if crumb.href}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- callers build crumb.href with $app/paths' resolve() -->
						<a href={crumb.href} class="hover:text-ink hover:underline">{crumb.label}</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{:else}
						{crumb.label}
					{/if}
				{/each}
			</p>
		{/if}
		<div class="flex flex-wrap items-end justify-between gap-4 border-b border-rule pb-5">
			<div class="min-w-0">
				<h1 class="text-3xl font-semibold">{title}</h1>
				{#if lede}<p class="mt-1.5 text-sm text-ink-muted">{lede}</p>{/if}
			</div>
			{#if action}<div class="flex items-center gap-2">{@render action()}</div>{/if}
		</div>
		<div class="mt-8">
			{@render children()}
		</div>
	</div>
</main>
