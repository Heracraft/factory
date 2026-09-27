<!--
  A count as a row of squares, one per unit: Isotype's own form for a
  quantity. The pricing cards show their vCPUs with it (2, 4, 8), so the
  sizes compare at a glance and the count is the count, not a share of
  something. Decorative: the number is in the text beside it.
-->
<script lang="ts">
	let { count }: { count: number } = $props();
</script>

<span class="units" aria-hidden="true">
	{#each Array.from({ length: count }, (_, i) => i) as i (i)}
		<i></i>
	{/each}
</span>

<style>
	.units {
		display: inline-grid;
		grid-auto-flow: column;
		gap: 3px;
	}
	i {
		display: block;
		width: 9px;
		height: 9px;
		border-radius: 1px;
		background: var(--sh-grey);
	}
</style>
