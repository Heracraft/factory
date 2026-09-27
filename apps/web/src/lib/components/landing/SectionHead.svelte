<!--
  The head every landing section shares: its number and a one-word label
  in the mono small caps of a drawing's running head, the bold serif title
  and one sentence. No shapes: a shape on the landing encodes something,
  and a head has nothing for one to say (docs/LANDING.md, "Shape
  language" and "The page's grammar").
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		index,
		label,
		title,
		id,
		children
	}: { index: string; label: string; title: string; id?: string; children?: Snippet } = $props();
</script>

<header class="sec-head inset">
	<p class="sec-index" aria-hidden="true"><b>{index}</b>{label}</p>
	<h2 {id} class="landing-h scroll-mt-6">{title}</h2>
	{#if children}
		<p class="lead">{@render children()}</p>
	{/if}
</header>
