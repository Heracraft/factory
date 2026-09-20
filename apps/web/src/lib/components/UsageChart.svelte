<script lang="ts">
	// A stacked bar chart, one bar per day, stacked by size class, of
	// guest-hours across every project this month (08-dashboard.md 5.2). The
	// three categorical colors are the palette skill's first three slots,
	// which validate all-pairs in both themes — safe for exactly the three
	// classes this chart will ever have.
	import type { SizeClass } from '$lib/api/types';

	let { rows }: { rows: Array<{ day: string; small: number; large: number; xl: number }> } =
		$props();

	const CLASSES: SizeClass[] = ['small', 'large', 'xl'];
	const WIDTH = 640;
	const HEIGHT = 160;
	const PAD_LEFT = 32;
	const PAD_BOTTOM = 18;

	let maxTotal = $derived(Math.max(1, ...rows.map((r) => r.small + r.large + r.xl)));
	let plotHeight = $derived(HEIGHT - PAD_BOTTOM);
	let barWidth = $derived(rows.length ? (WIDTH - PAD_LEFT) / rows.length : 0);

	let hover = $state<{ day: string; cls: SizeClass; hours: number; x: number; y: number } | undefined>(
		undefined
	);

	function segments(row: (typeof rows)[number], i: number) {
		let y = plotHeight;
		return CLASSES.map((cls) => {
			const hours = row[cls];
			const h = (hours / maxTotal) * (plotHeight - 8);
			y -= h;
			const seg = { cls, hours, x: PAD_LEFT + i * barWidth + 1, y, w: Math.max(0, barWidth - 2), h };
			y -= h > 0 ? 2 : 0; // a 2px surface gap between stacked segments
			return seg;
		});
	}
</script>

<div class="viz-root">
	<svg viewBox="0 0 {WIDTH} {HEIGHT}" role="img" aria-label="Guest-hours per day by class">
		<line
			x1={PAD_LEFT}
			y1={plotHeight}
			x2={WIDTH}
			y2={plotHeight}
			stroke="var(--viz-axis)"
			stroke-width="1"
		/>
		{#each rows as row, i (row.day)}
			{#each segments(row, i) as seg (seg.cls)}
				{#if seg.h > 0}
					<rect
						x={seg.x}
						y={seg.y}
						width={seg.w}
						height={seg.h}
						rx="2"
						fill="var(--viz-{seg.cls})"
						onmouseenter={() =>
							(hover = { day: row.day, cls: seg.cls, hours: seg.hours, x: seg.x, y: seg.y })}
						onmouseleave={() => (hover = undefined)}
					/>
				{/if}
			{/each}
		{/each}
	</svg>

	{#if hover}
		<div
			class="pointer-events-none absolute rounded-md border border-zinc-200 bg-white px-2 py-1 text-xs shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
			style="left: {(hover.x / WIDTH) * 100}%; top: {(hover.y / HEIGHT) * 100}%; transform: translate(-50%, -110%);"
		>
			{hover.day} · {hover.cls}: {hover.hours.toFixed(1)}h
		</div>
	{/if}

	<div class="mt-2 flex gap-4 text-xs text-zinc-500 dark:text-zinc-400">
		{#each CLASSES as cls (cls)}
			<span class="inline-flex items-center gap-1.5">
				<span class="inline-block h-2.5 w-2.5 rounded-sm" style="background: var(--viz-{cls})"
				></span>
				{cls}
			</span>
		{/each}
	</div>
</div>

<style>
	.viz-root {
		position: relative;
		color-scheme: light;
		--viz-axis: #d4d4d8;
		--viz-small: #2a78d6;
		--viz-large: #eb6834;
		--viz-xl: #1baf7a;
	}
	@media (prefers-color-scheme: dark) {
		:root:not([data-theme='light']) .viz-root {
			color-scheme: dark;
			--viz-axis: #3f3f46;
			--viz-small: #3987e5;
			--viz-large: #d95926;
			--viz-xl: #199e70;
		}
	}
	:global(:root[data-theme='dark']) .viz-root {
		color-scheme: dark;
		--viz-axis: #3f3f46;
		--viz-small: #3987e5;
		--viz-large: #d95926;
		--viz-xl: #199e70;
	}
</style>
