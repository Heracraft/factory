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
		action,
		children
	}: {
		title: string;
		crumbs?: Crumb[];
		width?: 'form' | 'list';
		action?: Snippet;
		children: Snippet;
	} = $props();
</script>

<div class="mx-auto {width === 'form' ? 'max-w-2xl' : 'max-w-3xl'} px-5 pt-10 pb-20">
	{#if crumbs.length}
		<p class="mb-2 text-sm text-zinc-500 dark:text-zinc-400">
			{#each crumbs as crumb, i (crumb.label)}
				{#if i > 0}<span class="mx-1.5">·</span>{/if}
				{#if crumb.href}<a href={crumb.href} class="link">{crumb.label}</a>{:else}{crumb.label}{/if}
			{/each}
		</p>
	{/if}
	<div class="flex items-start justify-between gap-4">
		<h1 class="font-display text-2xl font-semibold">{title}</h1>
		{#if action}<div class="pt-1">{@render action()}</div>{/if}
	</div>
	<div class="mt-6">
		{@render children()}
	</div>
</div>
