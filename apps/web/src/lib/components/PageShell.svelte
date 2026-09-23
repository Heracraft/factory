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
     what a screen reader's "skip to main content" jumps to. -->
<main class="mx-auto {width === 'form' ? 'max-w-2xl' : 'max-w-5xl'} px-5 pt-10 pb-24">
	{#if crumbs.length}
		<p class="mb-3 text-sm text-zinc-500 dark:text-zinc-400">
			{#each crumbs as crumb, i (crumb.label)}
				{#if i > 0}<span class="mx-1.5" aria-hidden="true">/</span>{/if}
				{#if crumb.href}
					<!-- eslint-disable svelte/no-navigation-without-resolve -- callers build crumb.href with $app/paths' resolve() -->
					<a href={crumb.href} class="hover:text-zinc-900 hover:underline dark:hover:text-zinc-100"
						>{crumb.label}</a
					>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{:else}
					{crumb.label}
				{/if}
			{/each}
		</p>
	{/if}
	<div class="flex flex-wrap items-end justify-between gap-4 border-b border-[var(--rule)] pb-5">
		<div class="min-w-0">
			<h1 class="text-3xl font-semibold">{title}</h1>
			{#if lede}<p class="mt-1.5 text-sm text-zinc-500 dark:text-zinc-400">{lede}</p>{/if}
		</div>
		{#if action}<div class="flex items-center gap-2">{@render action()}</div>{/if}
	</div>
	<div class="mt-8">
		{@render children()}
	</div>
</main>
