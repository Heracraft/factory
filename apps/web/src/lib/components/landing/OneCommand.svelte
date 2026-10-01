<!--
  One command, your working state. Two source-control panels, drawn (no
  vendor's chrome), listing real data from 2026-09-25: the laptop checkout
  of the owner's job-alerts app (git -C .../landing/recruiting log/status)
  and its machine after `repose run --no-attach`, whose real output was
  "Synced: 2 modified, 1 untracked, 1 env file (2 new commits)". On the
  machine the edits sit under Changes exactly as on the laptop: the sync
  sends staged and unstaged work as two patches and keeps the split (I-258,
  internal/cli/sync.go); the untracked test file stays untracked.
  Dependency directories never travel (docs sync.md "What doesn't"), so the
  laptop's node_modules/ is struck and stays behind, and the machine shows
  none: the run gives it no node_modules of its own. Since I-367 run syncs
  only into a new machine, so the chip reads the quickstart's "Ready in 14s"
  (docs index.md), the time of a run that creates the machine (I-397).

  `animated` (set by routes/+page.svelte) plays the sync with anime.js: the
  laptop's new commits and changed files lift off and travel into the
  machine's panel, staggered, then it rests on the synced state. Without
  it, or under prefers-reduced-motion, the synced state is shown still; a
  visitor who turns reduced motion on mid-loop gets the still frame at
  once. anime.js is only loaded when animated and motion is allowed.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { watchReducedMotion } from './inview';

	let { animated = false }: { animated?: boolean } = $props();

	type Commit = { msg: string; hash: string; base?: boolean };
	type File = { name: string; dir?: string; st: 'M' | 'U' | 'I' };

	const commits: Commit[] = [
		{ msg: 'Let fetchBoard take a timeout instead of the fixed 20 seconds', hash: 'a1dd18e' },
		{ msg: 'Make the board fetch timeout a worker setting', hash: 'cc300c3' },
		{
			msg: 'Drop the helper line under the audience switch; no hand-holding in the UI',
			hash: '7ea43a8',
			base: true
		}
	];
	const envExample: File = { name: '.env.example', st: 'M' };
	const poller: File = { name: 'poller.ts', dir: 'apps/worker/src/ingest', st: 'M' };
	const test: File = { name: 'timeout.test.ts', dir: 'apps/worker/src/ingest', st: 'U' };
	const env: File = { name: '.env', st: 'I' };

	const label = 'Your laptop and your cloud machine after repose run';
	const story =
		'Two source control panels side by side, your laptop and your cloud machine. On the laptop, branch fetch-timeout ' +
		'has 2 new commits, 2 modified files (.env.example and poller.ts), an untracked timeout.test.ts and a gitignored .env; ' +
		'its node_modules folder is struck out and stays behind. After repose run, ready in 14 seconds, the cloud machine shows ' +
		'the same branch, the same commits, the same two modified files and untracked test, and the same .env.';
	const descId = 'one-command-desc';

	let pic: HTMLDivElement;
	let fly: HTMLDivElement;
	let fire = $state(false);

	onMount(() => {
		if (!animated) return;

		let dead = false;
		let visible = false;
		let tl: { pause(): unknown; play(): unknown } | null = null;
		let io: IntersectionObserver | undefined;
		// Bumped on each start and stop, so an anime.js import that resolves
		// after a stop does not start a loop.
		let run = 0;

		const q = (s: string) => Array.from(pic.querySelectorAll<HTMLElement>(s));
		const arrivals = () => q('[data-to]');
		const later = () => q('.m-later, .m-grp');
		const moved = () => [
			...arrivals(),
			...later(),
			...q('.l-stop .strike, .l-stop .stop, .l-stop .fname, .l-stop .fi')
		];

		// Stop the loop and show the synced state still: the markup's own
		// state, so every inline style anime.js wrote comes off.
		function still() {
			run++;
			io?.disconnect();
			io = undefined;
			tl?.pause();
			tl = null;
			visible = false;
			// eslint-disable-next-line svelte/no-dom-manipulating
			fly.replaceChildren();
			fly.style.removeProperty('clip-path');
			for (const el of moved()) el.removeAttribute('style');
			fire = false;
		}

		function start() {
			const me = ++run;
			import('animejs').then(({ createTimeline, utils, stagger }) => {
				if (dead || me !== run) return;

				// Before the sync: the machine has no branch work yet, the
				// laptop's node_modules is not yet marked, nothing is ready.
				function pre() {
					utils.set([...arrivals(), ...later()], { opacity: 0 });
					utils.set(q('.l-stop .strike'), { scaleX: 0 });
					utils.set(q('.l-stop .stop'), { opacity: 0, scale: 0.6 });
					utils.set(q('.l-stop .fname, .l-stop .fi'), { opacity: 1 });
					fire = false;
				}
				pre();

				function cycle() {
					if (dead || me !== run) return;
					if (!visible) {
						tl = null;
						return;
					}
					// .fly is an empty layer Svelte never renders into; only these
					// throwaway copies of the laptop's rows live in it.
					// eslint-disable-next-line svelte/no-dom-manipulating
					fly.replaceChildren();
					pre();

					const base = pic.getBoundingClientRect();
					const [laptop, machine] = q('.side').map((el) => el.getBoundingClientRect());
					const clones: HTMLElement[] = [];
					const dx: number[] = [];
					const dy: number[] = [];
					const targets: HTMLElement[] = [];
					for (const from of q('[data-from]')) {
						const to = pic.querySelector<HTMLElement>(`[data-to="${from.dataset.from}"]`);
						if (!to) continue;
						const a = from.getBoundingClientRect();
						const b = to.getBoundingClientRect();
						const c = from.cloneNode(true) as HTMLElement;
						c.removeAttribute('data-from');
						c.classList.add('clone');
						Object.assign(c.style, {
							left: `${a.left - base.left}px`,
							top: `${a.top - base.top}px`,
							width: `${a.width}px`,
							opacity: '0'
						});
						// eslint-disable-next-line svelte/no-dom-manipulating
						fly.appendChild(c);
						clones.push(c);
						dx.push(b.left - a.left);
						dy.push(b.top - a.top);
						targets.push(to);
					}

					// Stacked (phone): the lower rows go first so no row overtakes
					// another on the way down. Side by side: top to bottom.
					const n = clones.length;
					const vertical = machine.top >= laptop.bottom;
					const order = clones.map((_, i) => (vertical ? n - 1 - i : i));
					// Stacked, a copy's path runs over the laptop's lower rows and
					// the machine's upper ones; it is seen only in the gap between
					// the two panels, sliding out of one and into the other.
					if (vertical)
						fly.style.clipPath = `inset(${laptop.bottom - base.top}px 0 ${base.bottom - machine.top}px 0)`;
					else fly.style.removeProperty('clip-path');

					const t0 = 900;
					const go = t0 + 260;
					const step = 110;
					const dur = 950;
					const landed = go + step * (n - 1) + dur;

					const t = createTimeline({ autoplay: false, onComplete: () => cycle() })
						.call(() => (fire = true), t0)
						.call(() => (fire = false), t0 + 520)
						.add(
							q('.l-stop .strike'),
							{ scaleX: [0, 1], duration: 380, ease: 'outCubic' },
							go + 140
						)
						.add(
							q('.l-stop .stop'),
							{ opacity: [0, 1], scale: [0.6, 1], duration: 300, ease: 'outBack' },
							go + 420
						)
						.add(q('.m-head'), { opacity: [0, 1], duration: 300 }, go + 200);

					clones.forEach((c, i) => {
						const start = go + step * order[i];
						t.set(c, { opacity: 1 }, start)
							.add(c, { x: dx[i], y: dy[i], duration: dur, ease: 'inOutCubic' }, start)
							.set(targets[i], { opacity: 1 }, start + dur)
							.set(c, { opacity: 0 }, start + dur + 16);
					});

					t.add(q('.m-grp'), { opacity: [0, 1], duration: 300, delay: stagger(90) }, go + 380)
						.add(q('.ready'), { opacity: [0, 1], duration: 400 }, landed + 150)
						// rest on the synced state, then clear the machine and go again
						.add(
							[...arrivals(), ...later()],
							{ opacity: 0, duration: 450, ease: 'inQuad' },
							landed + 7700
						)
						.add(q('.l-stop .strike'), { scaleX: 0, duration: 300, ease: 'inQuad' }, landed + 7750)
						.add(q('.l-stop .stop'), { opacity: 0, duration: 250 }, landed + 7750)
						.add({ duration: 400 }, landed + 8150);

					tl = t;
					t.play();
				}

				io = new IntersectionObserver(
					([e]) => {
						visible = e.isIntersecting;
						if (visible) {
							if (tl) tl.play();
							else cycle();
						} else {
							tl?.pause();
						}
					},
					{ threshold: 0.5 }
				);
				// Start when the machine's panel (the destination) is in view; on a
				// phone it sits below the laptop's.
				io.observe(pic.querySelectorAll('.side')[1]);
			});
		}

		const unwatch = watchReducedMotion((reduce) => {
			if (reduce) still();
			else start();
		});

		return () => {
			dead = true;
			unwatch();
			io?.disconnect();
			tl?.pause();
		};
	});
</script>

{#snippet branchIcon()}
	<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
		<g fill="none" stroke="currentColor" stroke-width="1.3">
			<circle cx="4.5" cy="3.5" r="1.6" />
			<circle cx="4.5" cy="12.5" r="1.6" />
			<circle cx="11.5" cy="5" r="1.6" />
			<path d="M4.5 5.1v5.8M11.5 6.6c0 3-7 2-7 4.3" />
		</g>
	</svg>
{/snippet}

{#snippet fileIcon()}
	<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="fi">
		<path
			d="M4 1.5h5.2L12.5 4.8v9.7H4z M9 1.5v3.5h3.5"
			fill="none"
			stroke="currentColor"
			stroke-width="1.1"
			stroke-linejoin="round"
		/>
	</svg>
{/snippet}

{#snippet folderIcon()}
	<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="fi">
		<path
			d="M1.5 3.5h4.3l1.4 1.6h7.3v8.4h-13z"
			fill="none"
			stroke="currentColor"
			stroke-width="1.1"
			stroke-linejoin="round"
		/>
	</svg>
{/snippet}

{#snippet laptopMark()}
	<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" class="mark">
		<g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round">
			<rect x="4.5" y="5" width="15" height="10.5" rx="1" />
			<path d="M2 18.5h20" stroke-linecap="round" />
		</g>
	</svg>
{/snippet}

{#snippet cloudMark()}
	<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" class="mark">
		<path
			d="M7 18.5h10.5a4 4 0 0 0 .6-7.96A5.5 5.5 0 0 0 7.4 9.1 4.7 4.7 0 0 0 7 18.5z"
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			stroke-linejoin="round"
		/>
	</svg>
{/snippet}

{#snippet file(f: File, side: 'laptop' | 'machine')}
	<li
		class="fr"
		class:ign={f.st === 'I'}
		data-from={side === 'laptop' ? f.name : undefined}
		data-to={side === 'machine' ? f.name : undefined}
	>
		{@render fileIcon()}
		<span class="fname">{f.name}</span>
		{#if f.dir}<span class="dir">{f.dir}</span>{/if}
		{#if f.st !== 'I'}<span class="st st-{f.st}">{f.st}</span>{/if}
	</li>
{/snippet}

{#snippet group(title: string, count: number | null, later = false)}
	<li class="grp" class:m-grp={later}>
		<svg viewBox="0 0 10 10" width="9" height="9" aria-hidden="true" class="chev">
			<path d="M2 3.5l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.3" />
		</svg>
		<span>{title}</span>
		{#if count !== null}<span class="count">{count}</span>{/if}
	</li>
{/snippet}

{#snippet panel(where: 'laptop' | 'machine')}
	{@const m = where === 'machine'}
	<div class="win">
		<div class="title">
			{#if m}
				{@render cloudMark()}<span class="who">your cloud machine</span><i class="dot"></i>
			{:else}
				{@render laptopMark()}<span class="who">your laptop</span>
			{/if}
		</div>
		<div class="scm">
			<div class="head">
				<span class="h">Source control</span>
				<span class="branch" class:m-head={m} class:m-later={m}
					>{@render branchIcon()}fetch-timeout</span
				>
			</div>

			<ol class="graph">
				{#each commits as c (c.hash)}
					<li
						class="commit"
						class:base={c.base}
						data-from={!m && !c.base ? c.hash : undefined}
						data-to={m && !c.base ? c.hash : undefined}
					>
						<i class="node"></i>
						<span class="msg">{c.msg}</span>
						{#if c.base}<span class="ref">master</span>{/if}
						<span class="hash">{c.hash}</span>
					</li>
				{/each}
			</ol>

			<ul class="files">
				{#if !m}
					{@render group('Changes', 3)}
					{@render file(envExample, where)}
					{@render file(poller, where)}
					{@render file(test, where)}
					{@render group('Ignored', null)}
					{@render file(env, where)}
					<li class="fr nm l-stop">
						{@render folderIcon()}
						<span class="fname">node_modules/<i class="strike"></i></span>
						<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="stop">
							<g fill="none" stroke="currentColor" stroke-width="1.5">
								<circle cx="8" cy="8" r="6" />
								<path d="M3.8 12.2l8.4-8.4" />
							</g>
						</svg>
					</li>
				{:else}
					{@render group('Changes', 3, true)}
					{@render file(envExample, where)}
					{@render file(poller, where)}
					{@render file(test, where)}
					{@render group('Ignored', null, true)}
					{@render file(env, where)}
				{/if}
			</ul>
		</div>
	</div>
{/snippet}

<div class="oc">
	<p id={descId} class="sr-only">{story}</p>
	<div class="pic" role="img" aria-label={label} aria-describedby={descId} bind:this={pic}>
		<div class="side" aria-hidden="true">{@render panel('laptop')}</div>

		<div class="hop" aria-hidden="true">
			<span class="wire"></span>
			<span class="chip" class:fire>repose run</span>
			<span class="wire arrow"></span>
			<span class="ready m-later">Ready in 14s</span>
		</div>

		<div class="side" aria-hidden="true">{@render panel('machine')}</div>

		<div class="fly" aria-hidden="true" bind:this={fly}></div>
	</div>
</div>

<style>
	/* Colours are the picture tokens (--pic-*, routes/layout.css), which
	   carry their own dark values. */
	.pic {
		position: relative;
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		min-width: 0;
	}
	.side {
		display: flex;
		min-width: 0;
	}

	/* Window */
	.win {
		flex: 1;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--rule-strong);
		border-radius: 3px;
		background: var(--surface);
		overflow: hidden;
		font-size: 13px;
		color: var(--pic-ink);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 36px;
		padding: 0 14px;
		border-bottom: 1px solid var(--rule);
		background: var(--sunken);
		font-size: 13px;
	}
	.mark {
		flex: none;
		color: var(--pic-ink);
	}
	.who {
		font-weight: 600;
	}
	.dot {
		width: 7px;
		height: 7px;
		margin-left: auto;
		border-radius: 50%;
		background: var(--color-emerald-500);
	}
	.scm {
		flex: 1;
		min-width: 0;
		padding: 10px 0 12px;
	}
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		padding: 0 14px 8px;
	}
	.h {
		font-weight: 600;
	}
	.branch {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--pic-ink);
	}

	/* Commit graph */
	.graph {
		position: relative;
		margin: 0 0 6px;
		padding: 0;
		list-style: none;
	}
	.commit {
		position: relative;
		display: flex;
		align-items: center;
		gap: 8px;
		height: 26px;
		padding: 0 14px 0 34px;
	}
	.node {
		position: absolute;
		left: 20px;
		top: 50%;
		width: 9px;
		height: 9px;
		margin-top: -4.5px;
		border-radius: 50%;
		background: var(--pic-accent);
		z-index: 1;
	}
	.graph .commit:not(:last-child)::after {
		content: '';
		position: absolute;
		left: 24px;
		top: 13px;
		height: 26px;
		border-left: 1.5px solid var(--pic-accent);
	}
	.commit.base .node {
		background: var(--surface);
		border: 1.5px solid var(--pic-faint);
	}
	.msg {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.base .msg {
		color: var(--pic-dim);
	}
	.hash,
	.ref {
		flex: none;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--pic-dim);
	}
	.ref {
		padding: 0 4px;
		border: 1px solid var(--rule-strong);
		border-radius: 2px;
		line-height: 16px;
	}

	/* Files */
	.files {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.grp {
		display: flex;
		align-items: center;
		gap: 5px;
		height: 26px;
		padding: 0 14px 0 12px;
		font-size: 12px;
		font-weight: 600;
		color: var(--pic-dim);
	}
	.chev {
		flex: none;
	}
	.count {
		margin-left: auto;
		min-width: 18px;
		padding: 0 5px;
		border-radius: 2px;
		background: var(--sunken);
		border: 1px solid var(--rule);
		text-align: center;
		font-weight: 500;
		line-height: 16px;
	}
	.fr {
		position: relative;
		display: flex;
		align-items: center;
		gap: 7px;
		height: 26px;
		padding: 0 14px 0 26px;
		white-space: nowrap;
	}
	.fi {
		flex: none;
		color: var(--pic-dim);
	}
	.fname {
		flex: none;
		font-family: var(--font-mono);
		font-size: 12.5px;
	}
	.dir {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		font-size: 12px;
		color: var(--pic-dim);
	}
	.st {
		flex: none;
		width: 12px;
		margin-left: auto;
		text-align: center;
		font-family: var(--font-mono);
		font-size: 12px;
		font-weight: 600;
	}
	.st-M {
		color: var(--color-amber-600);
	}
	.st-U {
		color: var(--color-emerald-600);
	}
	.ign .fname {
		color: var(--pic-dim);
	}

	/* node_modules: the laptop's is struck and stays behind. Name and icon
	   in --pic-faint, which holds 4.5:1 on the panel; the strike and the
	   red stop sign say it does not travel. */
	.nm .fi,
	.nm .fname {
		color: var(--pic-faint);
	}
	.l-stop .fname {
		position: relative;
	}
	.strike {
		position: absolute;
		left: -2px;
		right: -2px;
		top: calc(50% - 2px);
		border-top: 1.5px solid var(--pic-stop);
		transform-origin: left center;
	}
	.stop {
		flex: none;
		margin-left: auto;
		color: var(--pic-stop);
	}

	/* The command between them */
	.hop {
		position: relative;
		z-index: 2;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 6px 0;
	}
	.wire {
		width: 0;
		height: 18px;
		border-left: 1.5px solid var(--pic-accent);
	}
	.wire.arrow {
		position: relative;
	}
	.wire.arrow::after {
		content: '';
		position: absolute;
		left: -5.5px;
		bottom: -1px;
		border: 5px solid transparent;
		border-top: 6px solid var(--pic-accent);
		border-bottom: 0;
	}
	.chip {
		padding: 3px 9px;
		border: 1px solid var(--pic-accent);
		border-radius: 3px;
		background: var(--surface);
		font-family: var(--font-mono);
		font-size: 13px;
		font-weight: 600;
		color: var(--pic-accent);
		white-space: nowrap;
		transition:
			background-color 0.18s,
			color 0.18s;
	}
	.chip.fire {
		background: var(--pic-accent);
		color: var(--surface);
	}
	.ready {
		position: absolute;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--color-zinc-600);
		display: inline-flex;
		align-items: center;
		gap: 6px;
		top: 50%;
		left: calc(50% + 62px);
		transform: translateY(-50%);
		/* Stacked, the chip sits right of "repose run" with only the half
		   frame beyond it: at 320px, or under WCAG 1.4.12 text spacing, the
		   line wraps inside the frame rather than spilling past the page. */
		max-width: calc(50% - 62px);
		white-space: normal;
	}
	.ready::before {
		content: '';
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-emerald-500);
	}

	/* Rows in flight (animated only): copies of the laptop's rows. */
	.fly {
		position: absolute;
		inset: 0;
		z-index: 1;
		pointer-events: none;
		font-size: 13px;
		color: var(--pic-ink);
	}
	.fly :global(.clone) {
		position: absolute;
		margin: 0;
		list-style: none;
		background: color-mix(in oklab, var(--pic-accent) 9%, var(--surface));
		box-shadow: inset 2px 0 0 var(--pic-accent);
		will-change: transform;
	}
	.fly :global(.clone.commit) {
		padding-right: 14px;
	}

	@media (min-width: 768px) {
		.pic {
			grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
		}
		.hop {
			flex-direction: row;
			align-self: center;
			padding: 0 8px;
		}
		.wire {
			width: 14px;
			height: 0;
			border-left: 0;
			border-top: 1.5px solid var(--pic-accent);
		}
		.wire.arrow::after {
			left: auto;
			right: -1px;
			bottom: auto;
			top: -6.25px;
			border: 5px solid transparent;
			border-left: 6px solid var(--pic-accent);
			border-right: 0;
		}
		.ready {
			top: calc(50% + 24px);
			left: 50%;
			transform: translateX(-50%);
			max-width: none;
			white-space: nowrap;
		}
	}
	@media (max-width: 480px) {
		.win,
		.fly {
			font-size: 12px;
		}
		.fname {
			font-size: 11.5px;
		}
		.hash,
		.ref,
		.dir,
		.grp,
		.branch,
		.title,
		.st,
		.ready {
			font-size: 11px;
		}
	}
	@media (prefers-color-scheme: dark) {
		.st-M {
			color: var(--color-amber-400);
		}
		.st-U {
			color: var(--color-emerald-400);
		}
		.ready {
			color: var(--color-zinc-400);
		}
	}
</style>
