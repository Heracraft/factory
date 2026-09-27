<!--
  The twelve tiles beside the headline. They land one after another on
  load; one tile is the agent, its mark cycling through the five agents on
  every machine. The grid follows the hero picture below it (Hero.svelte
  calls onbeat): each snapshot clicks the grid, the wreck knocks every tile
  out of place and drains its colour, the restore springs them back. No
  words: the motion carries it, as docs/LANDING.md asks.
-->
<script lang="ts" module>
	export type Beat = 'run' | 'shot' | 'wreck' | 'restore' | 'out';
</script>

<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { agentMarks } from '$lib/components/illustrations/marks';
	import Shape, { type Kind } from './Shape.svelte';
	import { reducedMotion } from './inview';

	let { beat }: { beat?: { kind: Beat; n: number } } = $props();

	const tiles: (Kind | 'agent')[] = [
		'diamond',
		'pie',
		'pill',
		'agent',
		'halves',
		'asterisk',
		'ring',
		'star',
		'marble',
		'arch',
		'sphere',
		'leaf'
	];
	// How far a tile turns when it turns: what looks like a turn for its shape.
	const TURN: Partial<Record<Kind, number>> = { pie: 90, asterisk: 45, ring: 90, star: 45 };
	// A tumble per tile for the wreck, fixed so every loop falls the same way.
	const FALL = tiles.map((_, i) => {
		const r = Math.sin(i * 12.9898) * 43758.5453;
		const f = r - Math.floor(r);
		return { x: (f - 0.5) * 34, y: 10 + ((f * 7) % 1) * 38, r: (f - 0.5) * 70 };
	});

	let landed = $state(false);
	let wrecked = $state(false);
	let click = $state(0);
	let angle = $state(tiles.map(() => 0));
	let who = $state(0);

	// The marks don't fill their 24px box the same way (Claude Code's is
	// short and wide), so each is centred on its own drawn bounds and
	// scaled to fit a 44px square in the middle of the tile.
	function centre(g: SVGGElement) {
		const b = (g.firstElementChild as SVGGElement).getBBox();
		const k = 44 / Math.max(b.width, b.height);
		const x = 50 - (b.x + b.width / 2) * k;
		const y = 50 - (b.y + b.height / 2) * k;
		g.setAttribute('transform', `translate(${x} ${y}) scale(${k})`);
	}

	function turn(i: number) {
		const t = tiles[i];
		if (t !== 'agent' && TURN[t]) angle[i] += TURN[t]!;
	}

	$effect(() => {
		const b = beat;
		if (!b) return;
		untrack(() => {
			if (b.kind === 'shot') {
				click++;
				turn(tiles.indexOf('ring'));
			} else if (b.kind === 'wreck') wrecked = true;
			// A picture that restarts (a resize, a scroll back) begins with
			// 'run', so the grid is whole again even if it missed 'restore'.
			else wrecked = false;
		});
	});

	onMount(() => {
		if (reducedMotion()) {
			landed = true;
			return;
		}
		const raf = requestAnimationFrame(() => (landed = true));
		// At rest only the agent tile moves: the next agent's mark, every
		// few seconds. Turns come from a hover or a snapshot.
		const iv = setInterval(() => {
			if (!wrecked) who = (who + 1) % agentMarks.length;
		}, 3200);
		return () => {
			cancelAnimationFrame(raf);
			clearInterval(iv);
		};
	});
</script>

<div class="grid-3" class:wrecked aria-hidden="true">
	{#each tiles as t, i (i)}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="tile"
			class:landed
			style="--d: {(Math.floor(i / 3) + (i % 3)) * 85}ms; --a: {angle[i]}deg; --fx: {FALL[i]
				.x}%; --fy: {FALL[i].y}%; --fr: {FALL[i].r}deg; --w: {(i % 5) * 45}ms"
			onpointerenter={() => turn(i)}
		>
			<div class="fall">
				{#key click}<div class="spin" class:clicked={click > 0}>
						{#if t === 'agent'}
							<svg viewBox="0 0 100 100" class="block h-full w-full">
								<circle cx="50" cy="50" r="50" class="agent-bg" />
								{#key who}
									<g class="agent-mark" use:centre>
										<g class="agent-pop">
											{#each agentMarks[who].paths as d (d)}
												<path {d} fill-rule={agentMarks[who].evenodd ? 'evenodd' : 'nonzero'} />
											{/each}
										</g>
									</g>
								{/key}
							</svg>
						{:else}
							<Shape kind={t} />
						{/if}
					</div>{/key}
			</div>
		</div>
	{/each}
</div>

<style>
	.grid-3 {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: clamp(10px, 1.6vw, 22px);
	}
	/* On a phone the grid lies down: two rows of six under the headline. */
	@media (max-width: 767px) {
		.grid-3 {
			grid-template-columns: repeat(6, minmax(0, 1fr));
		}
	}
	.tile {
		aspect-ratio: 1;
		opacity: 0;
		transform: translateY(22%) scale(0.55);
		transition:
			opacity 0.35s ease-out var(--d),
			transform 0.75s cubic-bezier(0.34, 1.56, 0.64, 1) var(--d);
	}
	.tile.landed {
		opacity: 1;
		transform: none;
	}
	.fall {
		width: 100%;
		height: 100%;
		transition:
			transform 0.8s cubic-bezier(0.34, 1.45, 0.64, 1) var(--w),
			filter 0.6s ease-out var(--w),
			opacity 0.6s ease-out var(--w);
	}
	.wrecked .fall {
		transform: translate(var(--fx), var(--fy)) rotate(var(--fr));
		filter: grayscale(1);
		opacity: 0.5;
		transition:
			transform 0.55s cubic-bezier(0.55, 0, 0.9, 0.5) var(--w),
			filter 0.4s ease-in var(--w),
			opacity 0.4s ease-in var(--w);
	}
	.spin {
		width: 100%;
		height: 100%;
		transform: rotate(var(--a));
		transition: transform 0.9s cubic-bezier(0.65, 0, 0.35, 1);
	}
	/* A snapshot being taken: every tile gives one short click. */
	.clicked {
		animation: click 0.42s cubic-bezier(0.3, 0, 0.3, 1) calc(var(--d) * 0.35);
	}
	@keyframes click {
		35% {
			scale: 0.86;
		}
	}
	.agent-bg {
		fill: var(--sh-paper);
		transition: fill 0.3s;
	}
	.agent-mark {
		fill: var(--sh-ink);
	}
	/* The pop is on the inner group: a transform-origin on the outer one
	   would also move the transform that centres the mark. */
	.agent-pop {
		animation: mark-in 0.45s cubic-bezier(0.34, 1.56, 0.64, 1);
		transform-box: fill-box;
		transform-origin: center;
	}
	/* The agent turns red when it goes rogue, as it does in the picture. */
	.wrecked .agent-bg {
		fill: var(--sh-red);
	}
	.wrecked .tile:has(.agent-bg) .fall {
		filter: none;
		opacity: 1;
	}
	@keyframes mark-in {
		from {
			opacity: 0;
			scale: 0.4;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tile,
		.fall,
		.spin {
			transition: none;
		}
		.clicked,
		.agent-pop {
			animation: none;
		}
	}
</style>
