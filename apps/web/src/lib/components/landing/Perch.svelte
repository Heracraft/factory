<!--
  A row of shapes standing on the top edge of the panel it's placed in (the
  panel must be position: relative), the way the footer's shapes stand on
  the footer's rule and the pricing shapes sit in their card's corner:
  shapes are anchored to the page's structure, never left floating
  (docs/LANDING.md, "Shape language").

  They rise out of the edge when it comes into view and turn a quarter on
  hover. On the hero (`beat`, from Hero.svelte's onbeat) they hop at each
  snapshot, are knocked over at the wreck (the agent tile turns red), and
  stand back up at the restore; the agent tile cycles the five agents'
  marks. Nothing moves on its own otherwise.
-->
<script lang="ts" module>
	export type Beat = 'run' | 'shot' | 'wreck' | 'restore' | 'out';
</script>

<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { agentMarks } from '$lib/components/illustrations/marks';
	import Shape, { type Kind, type Tone } from './Shape.svelte';
	import { landOnView, reducedMotion } from './inview';

	let {
		shapes,
		beat,
		large = false
	}: {
		// A shape, or "shape:tone" to pick its tone (Shape.svelte).
		shapes: (Kind | 'agent' | `${Kind}:${Tone}`)[];
		beat?: { kind: Beat; n: number };
		large?: boolean;
	} = $props();

	const split = (s: string) => s.split(':') as [Kind | 'agent', Tone | undefined];

	const TURN: Partial<Record<Kind, number>> = {
		pie: 90,
		asterisk: 45,
		ring: 90,
		star: 45,
		pinwheel: 90,
		burst: 15
	};

	// Quarter turns per tile, grown on demand (a missing entry is 0).
	let angle: number[] = $state([]);
	let down = $state(false);
	let hop = $state(0);
	let who = $state(0);

	function turn(i: number) {
		const k = split(shapes[i])[0];
		if (k !== 'agent') angle[i] = (angle[i] ?? 0) + (TURN[k] ?? 90);
	}

	$effect(() => {
		const b = beat;
		if (!b) return;
		untrack(() => {
			if (b.kind === 'shot') hop++;
			else if (b.kind === 'wreck') down = true;
			// A picture that restarts (a resize, a scroll back) begins with
			// 'run', so the row stands again even if it missed 'restore'.
			else down = false;
		});
	});

	onMount(() => {
		if (!shapes.includes('agent') || reducedMotion()) return;
		const iv = setInterval(() => {
			if (!down) who = (who + 1) % agentMarks.length;
		}, 3200);
		return () => clearInterval(iv);
	});

	// The marks don't fill their 24px box the same way (Claude Code's is
	// short and wide), so each is centred on its own drawn bounds and fitted
	// to a 44px square in the middle of the tile.
	function centre(g: SVGGElement) {
		const b = (g.firstElementChild as SVGGElement).getBBox();
		const k = 44 / Math.max(b.width, b.height);
		g.setAttribute(
			'transform',
			`translate(${50 - (b.x + b.width / 2) * k} ${50 - (b.y + b.height / 2) * k}) scale(${k})`
		);
	}
</script>

<div class="perch" class:large class:down aria-hidden="true" use:landOnView>
	{#each shapes.map(split) as [k, tone], i (i)}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<span
			class="tile land"
			class:agent={k === 'agent'}
			style="--d: {i * 90}ms; --tip: {i % 2 ? 1 : -1}"
			onpointerenter={() => turn(i)}
		>
			<span class="fall">
				{#key hop}<span class="turn hop" class:hopped={hop > 0} style="--a: {angle[i] ?? 0}deg">
						{#if k === 'agent'}
							<svg viewBox="0 0 100 100" class="block h-full w-full overflow-visible">
								<circle cx="50" cy="50" r="49.5" class="agent-bg" />
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
							<Shape kind={k} {tone} />
						{/if}
					</span>{/key}
			</span>
		</span>
	{/each}
</div>

<style>
	.perch {
		position: absolute;
		right: clamp(1rem, 3vw, 2rem);
		bottom: calc(100% + 1px);
		display: flex;
		align-items: flex-end;
		gap: 0.625rem;
		pointer-events: auto;
	}
	.tile {
		display: block;
		width: 2.25rem;
		height: 2.25rem;
	}
	.large .tile {
		width: 2.75rem;
		height: 2.75rem;
	}
	@media (max-width: 639px) {
		.tile,
		.large .tile {
			width: 1.75rem;
			height: 1.75rem;
		}
		.perch {
			gap: 0.5rem;
		}
	}
	.fall,
	.hop {
		display: block;
		width: 100%;
		height: 100%;
	}
	.fall {
		transform-origin: 50% 100%;
		transition:
			transform 0.8s cubic-bezier(0.34, 1.45, 0.64, 1) var(--d),
			filter 0.5s ease-out var(--d),
			opacity 0.5s ease-out var(--d);
	}
	/* Knocked over: each tips off its base, the way it stands. */
	.down .fall {
		transform: rotate(calc(var(--tip) * 72deg)) translateY(8%);
		filter: grayscale(1);
		opacity: 0.45;
		transition:
			transform 0.5s cubic-bezier(0.55, 0, 0.9, 0.5) var(--d),
			filter 0.35s ease-in var(--d),
			opacity 0.35s ease-in var(--d);
	}
	.down .agent .fall {
		transform: none;
		filter: none;
		opacity: 1;
	}
	/* A snapshot being taken: every shape gives one small hop. */
	.hopped {
		animation: hop 0.45s cubic-bezier(0.3, 0, 0.3, 1) calc(var(--d) * 0.5);
	}
	@keyframes hop {
		40% {
			translate: 0 -22%;
		}
	}
	.agent-bg {
		fill: var(--sh-paper);
		stroke: var(--rule-strong);
		stroke-width: 1.5;
		transition:
			fill 0.3s,
			stroke 0.3s;
	}
	/* The agent's mark is ink on paper, and paper on ink in the dark, as
	   the marks are in the toolchain box. */
	.agent-mark {
		fill: var(--color-zinc-800);
		transition: fill 0.3s;
	}
	@media (prefers-color-scheme: dark) {
		.agent-mark {
			fill: var(--color-zinc-200);
		}
	}
	/* The agent turns red when it goes rogue, as it does in the picture. */
	.down .agent-bg {
		fill: var(--sh-stop);
		stroke: var(--sh-stop);
	}
	.down .agent-mark {
		fill: var(--surface);
	}
	/* The pop is on the inner group: a transform-origin on the outer one
	   would also move the transform that centres the mark. */
	.agent-pop {
		animation: mark-in 0.45s cubic-bezier(0.34, 1.56, 0.64, 1);
		transform-box: fill-box;
		transform-origin: center;
	}
	@keyframes mark-in {
		from {
			opacity: 0;
			scale: 0.4;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.fall {
			transition: none;
		}
		.hopped,
		.agent-pop {
			animation: none;
		}
	}
</style>
