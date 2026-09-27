<!--
  One share of a plan's limit as a thin bar: what is used of what the plan
  holds (memory running now, disk allocated, egress this period). One
  series, so no legend; the numbers beside it carry the reading and the
  bar is a picture of them. The fill is ink; past the limit it turns amber
  and the words say "over", so the state is never colour alone.
-->
<script lang="ts">
	import { share } from '$lib/format';

	let {
		label,
		used,
		limit,
		format,
		note
	}: {
		label: string;
		used: number;
		limit: number;
		/** Formats a figure for the "used of limit" text. */
		format: (n: number) => string;
		/** A line under the bar: the overage, or what the limit means. */
		note?: string;
	} = $props();

	let fraction = $derived(share(used, limit));
	let over = $derived(limit > 0 && used > limit);
</script>

<div class="meter" data-testid="meter-{label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}">
	<div class="flex items-baseline justify-between gap-4 text-sm">
		<span class="font-medium">{label}</span>
		<span class="font-mono text-[13px] text-zinc-600 dark:text-zinc-400">
			{format(used)} of {format(limit)}
		</span>
	</div>
	<div
		class="mt-1.5 h-1.5 w-full overflow-hidden rounded-xs"
		style="background-color: var(--sunken)"
		role="meter"
		aria-label={label}
		aria-valuemin="0"
		aria-valuemax={limit}
		aria-valuenow={Math.min(used, limit)}
	>
		<div
			class="h-full rounded-xs {over
				? 'bg-amber-600 dark:bg-amber-400'
				: 'bg-zinc-800 dark:bg-zinc-200'}"
			style="width: {fraction * 100}%"
		></div>
	</div>
	{#if note}
		<p class="mt-1.5 text-xs text-zinc-500 dark:text-zinc-400">{note}</p>
	{/if}
</div>
