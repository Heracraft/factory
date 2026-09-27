<!--
  The header every landing section shares: a bold serif heading and one
  sentence on the left and, for a section whose picture has no shapes of
  its own, a trio of shapes on the right (docs/LANDING.md, "Shape
  language"). The trio lands when the header comes into view and each
  shape turns a quarter on hover, the page's one motion for shapes.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Shape, { type Kind } from './Shape.svelte';
	import { landOnView } from './inview';

	let {
		title,
		shapes = [],
		id,
		children
	}: { title: string; shapes?: Kind[]; id?: string; children?: Snippet } = $props();

	let angle = $state([0, 0, 0]);
</script>

<header class="head">
	<div class="min-w-0">
		<h2 {id} class="landing-h scroll-mt-6">{title}</h2>
		{#if children}
			<p class="mt-3 max-w-2xl leading-relaxed text-zinc-600 dark:text-zinc-400">
				{@render children()}
			</p>
		{/if}
	</div>
	{#if shapes.length}
		<div class="trio" aria-hidden="true" use:landOnView>
			{#each shapes as k, i (i)}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<span class="tile land" style="--d: {i * 110}ms" onpointerenter={() => (angle[i] += 90)}>
					<span class="turn block h-full w-full" style="--a: {angle[i]}deg"><Shape kind={k} /></span
					>
				</span>
			{/each}
		</div>
	{/if}
</header>

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1.25rem 2.5rem;
	}
	.trio {
		display: flex;
		flex: none;
		gap: 0.75rem;
		padding-bottom: 0.25rem;
	}
	.tile {
		display: block;
		width: 2.75rem;
		height: 2.75rem;
	}
	/* On a phone the trio sits above the heading, smaller. */
	@media (max-width: 639px) {
		.head {
			flex-direction: column-reverse;
			align-items: flex-start;
		}
		.tile {
			width: 2rem;
			height: 2rem;
		}
	}
</style>
