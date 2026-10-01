<!--
  A count as a row of squares, one per unit: Isotype's own form for a
  quantity. The pricing cards show the memory that may run at once with
  it, one square per GB (8, 16, 32), so the plans compare at a glance and
  the count is the count, not a share of something. Rows hold eight, so 32 reads as a block of four rows and
  still fits a narrow card. Decorative: the number is in the text beside it.
-->
<script lang="ts">
	let { count, perRow = 8 }: { count: number; perRow?: number } = $props();
</script>

<span class="units" style="--cols: {Math.min(count, perRow)}" aria-hidden="true">
	{#each Array.from({ length: count }, (_, i) => i) as i (i)}
		<i></i>
	{/each}
</span>

<style>
	.units {
		display: inline-grid;
		grid-template-columns: repeat(var(--cols), 9px);
		gap: 3px;
	}
	i {
		display: block;
		width: 9px;
		height: 9px;
		border-radius: var(--radius-xs);
		background: var(--sh-grey);
	}
</style>
