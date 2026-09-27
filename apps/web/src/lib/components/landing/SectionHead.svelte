<!--
  The head every landing section shares: the bold serif title and one
  sentence, inset from the rails. No shapes and no running label: a shape
  on the landing encodes something, and a head has nothing for one to say
  (docs/LANDING.md, "Shape language" and "The page's grammar").
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { title, id, children }: { title: string; id?: string; children?: Snippet } = $props();
</script>

<header class="sec-head inset">
	<h2 {id} class="landing-h scroll-mt-6">{title}</h2>
	{#if children}
		<p class="lead">{@render children()}</p>
	{/if}
</header>
