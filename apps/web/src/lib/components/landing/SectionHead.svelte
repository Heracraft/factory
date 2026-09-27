<!--
  The header every landing section shares: a bold serif heading and one
  sentence. A section's shapes are not here: they stand on the edge of its
  picture's panel (Perch.svelte), so they are anchored to the page's
  structure instead of floating beside the words.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { title, id, children }: { title: string; id?: string; children?: Snippet } = $props();
</script>

<header>
	<h2 {id} class="landing-h scroll-mt-6">{title}</h2>
	{#if children}
		<p class="mt-3 max-w-2xl leading-relaxed text-zinc-600 dark:text-zinc-400">
			{@render children()}
		</p>
	{/if}
</header>
