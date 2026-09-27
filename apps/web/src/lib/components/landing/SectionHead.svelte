<!--
  The header every landing section shares: a bold serif heading and one
  sentence. No shapes here: a shape on the landing encodes something (the
  agents, a step's progress, a size's share), and a section header has
  nothing for one to encode (docs/LANDING.md, "Shape language").
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
