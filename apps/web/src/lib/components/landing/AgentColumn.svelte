<!--
  "Let your agents run": the agents themselves, beside the headline. The
  five that come on every machine (marks.ts), each a circle tile, standing
  on the hero picture's top edge as a column (a row on a phone), the one
  the picture shows (Claude Code) at the foot, nearest to it.

  That tile follows the picture (Hero.svelte's onbeat): it fills with its
  orange while the agent works, turns red when the agent goes rogue, and
  goes back to orange when the snapshot restores the machine; every tile
  gives one small hop when a snapshot is taken. The others stay as they
  are: they could be the one running. Nothing moves on its own.
-->
<script lang="ts" module>
	export type Beat = 'run' | 'shot' | 'wreck' | 'restore' | 'out';
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import { agentMarks } from '$lib/components/illustrations/marks';
	import { landOnView } from './inview';

	let { beat }: { beat?: { kind: Beat; n: number } } = $props();

	type State = 'idle' | 'working' | 'rogue';
	let mode: State = $state('idle');
	let hop = $state(0);

	$effect(() => {
		const b = beat;
		if (!b) return;
		untrack(() => {
			if (b.kind === 'shot') hop++;
			else if (b.kind === 'run' || b.kind === 'restore') mode = 'working';
			else if (b.kind === 'wreck') mode = 'rogue';
			else mode = 'idle';
		});
	});

	// The marks don't fill their 24px box the same way (Claude Code's is
	// short and wide), so each is centred on its own drawn bounds and fitted
	// to a 48px square in the middle of the tile.
	function centre(g: SVGGElement) {
		const b = (g.firstElementChild as SVGGElement).getBBox();
		const k = 48 / Math.max(b.width, b.height);
		g.setAttribute(
			'transform',
			`translate(${50 - (b.x + b.width / 2) * k} ${50 - (b.y + b.height / 2) * k}) scale(${k})`
		);
	}
</script>

<div class="agents" aria-hidden="true" use:landOnView>
	{#each agentMarks as m, i (m.name)}
		<span
			class="tile land"
			class:lead={i === 0}
			data-state={i === 0 ? mode : 'idle'}
			style="--d: {i * 80}ms"
			title={m.name}
		>
			{#key hop}
				<svg viewBox="0 0 100 100" class="hop" class:hopped={hop > 0}>
					<circle cx="50" cy="50" r="48.5" class="bg" />
					<g class="mark" use:centre>
						<g>
							{#each m.paths as d (d)}
								<path {d} fill-rule={m.evenodd ? 'evenodd' : 'nonzero'} />
							{/each}
						</g>
					</g>
				</svg>
			{/key}
		</span>
	{/each}
</div>

<style>
	.agents {
		position: absolute;
		right: 0;
		bottom: calc(100% + 1px);
		display: flex;
		flex-direction: row-reverse;
		align-items: flex-end;
		gap: 0.5rem;
	}
	@media (min-width: 768px) {
		.agents {
			flex-direction: column-reverse;
			gap: 0.625rem;
		}
	}
	.tile {
		display: block;
		width: 2rem;
		height: 2rem;
	}
	@media (min-width: 768px) {
		.tile {
			width: 2.75rem;
			height: 2.75rem;
		}
	}
	.hop {
		display: block;
		width: 100%;
		height: 100%;
		overflow: visible;
	}
	.bg {
		fill: var(--surface);
		stroke: var(--rule-strong);
		stroke-width: 2;
		transition:
			fill 0.35s,
			stroke 0.35s;
	}
	.mark {
		fill: var(--color-zinc-700);
		transition: fill 0.35s;
	}
	/* The agent the picture shows, at work: its own orange. */
	[data-state='working'] .bg {
		fill: var(--sh-warm);
		stroke: var(--sh-warm);
	}
	[data-state='working'] .mark,
	[data-state='rogue'] .mark {
		fill: var(--surface);
	}
	/* Gone rogue, as the picture's agent does. */
	[data-state='rogue'] .bg {
		fill: var(--sh-stop);
		stroke: var(--sh-stop);
	}
	@media (prefers-color-scheme: dark) {
		.mark {
			fill: var(--color-zinc-300);
		}
	}
	/* A snapshot being taken: every tile gives one small hop. */
	.hopped {
		animation: hop 0.45s cubic-bezier(0.3, 0, 0.3, 1) calc(var(--d) * 0.5);
	}
	@keyframes hop {
		40% {
			translate: 0 -18%;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.bg,
		.mark {
			transition: none;
		}
		.hopped {
			animation: none;
		}
	}
</style>
