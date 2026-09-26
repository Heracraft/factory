<!--
  One command, your working state. Two source-control panels, drawn (no
  vendor's chrome), listing real data from 2026-09-25: the laptop checkout
  of the owner's job-alerts app (git -C .../landing/recruiting log/status)
  and its machine after `repose run --no-attach`, whose real output was
  "Synced: 2 modified, 1 untracked, 1 env file (2 new commits)" and
  "Ready in 0.6s.". On the machine the edits are staged because the sync
  applies them with `git apply --index` (internal/cli/sync.go); the
  untracked test file stays untracked. The machine's node_modules/ is its
  own install (9 node_modules/ in its `git status --ignored`); dependency
  directories never travel (docs sync.md "What doesn't"), so the laptop's
  is struck and stays behind.

  `animated` (OneCommandAnimated.svelte sets it) plays the sync with
  anime.js: the laptop's new commits and changed files lift off and travel
  into the machine's panel, staggered, then it rests on the synced state.
  Without it, or under prefers-reduced-motion, the synced state is shown
  still. anime.js is only loaded when animated.
-->
<script lang="ts">
	import { onMount } from 'svelte';

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

	const label =
		'Two source control panels side by side, your laptop and your cloud machine. On the laptop, branch fetch-timeout ' +
		'has 2 new commits, 2 modified files (.env.example and poller.ts), an untracked timeout.test.ts and a gitignored .env; ' +
		'its node_modules folder is struck out. After repose run, ready in 0.6 seconds, the cloud machine shows the same branch, ' +
		'the same commits, the two edits staged, the same untracked test and .env, and a node_modules of its own.';

	let pic: HTMLDivElement;
	let fly: HTMLDivElement;
	let fire = $state(false);

	onMount(() => {
		if (!animated) return;
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

		let dead = false;
		let visible = false;
		let tl: { pause(): unknown; play(): unknown; revert(): unknown } | null = null;
		let io: IntersectionObserver | undefined;

		const q = (s: string) => Array.from(pic.querySelectorAll<HTMLElement>(s));
		const arrivals = () => q('[data-to]');
		const later = () => q('.m-later, .m-grp');

		// Before the sync: the machine has no branch work yet, the laptop's
		// node_modules is not yet marked, nothing is ready.
		function pre(utils: typeof import('animejs').utils) {
			utils.set([...arrivals(), ...later()], { opacity: 0 });
			utils.set(q('.l-stop .strike'), { scaleX: 0 });
			utils.set(q('.l-stop .stop'), { opacity: 0, scale: 0.6 });
			utils.set(q('.l-stop .fname, .l-stop .fi'), { opacity: 1 });
			fire = false;
		}

		import('animejs').then(({ createTimeline, utils, stagger }) => {
			if (dead) return;
			pre(utils);

			function cycle() {
				if (dead) return;
				if (!visible) {
					tl = null;
					return;
				}
				// .fly is an empty layer Svelte never renders into; only these
				// throwaway copies of the laptop's rows live in it.
				// eslint-disable-next-line svelte/no-dom-manipulating
				fly.replaceChildren();
				pre(utils);

				const base = pic.getBoundingClientRect();
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
				const vertical =
					dy.reduce((a, v) => a + Math.abs(v), 0) > dx.reduce((a, v) => a + Math.abs(v), 0);
				const order = clones.map((_, i) => (vertical ? n - 1 - i : i));

				const t0 = 900;
				const go = t0 + 260;
				const step = 110;
				const dur = 950;
				const landed = go + step * (n - 1) + dur;

				const t = createTimeline({ autoplay: false, onComplete: () => cycle() })
					.call(() => (fire = true), t0)
					.call(() => (fire = false), t0 + 520)
					.add(q('.l-stop .strike'), { scaleX: [0, 1], duration: 380, ease: 'outCubic' }, go + 140)
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
					.add(q('.m-nm'), { opacity: [0, 1], duration: 500 }, landed + 200)
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

		return () => {
			dead = true;
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
					{@render group('Staged changes', 2, true)}
					{@render file(envExample, where)}
					{@render file(poller, where)}
					{@render group('Changes', 1, true)}
					{@render file(test, where)}
					{@render group('Ignored', null, true)}
					{@render file(env, where)}
					<li class="fr nm m-nm m-later">
						{@render folderIcon()}
						<span class="fname">node_modules/</span>
					</li>
				{/if}
			</ul>
		</div>
	</div>
{/snippet}

<div class="oc">
	<h2 class="text-2xl font-semibold">Your working state, in one command</h2>
	<p class="mt-3 max-w-2xl leading-relaxed text-zinc-600 dark:text-zinc-400">
		Run <code
			class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
			>repose run</code
		> in any checkout and your cloud machine picks up where your laptop is, down to the uncommitted edits.
	</p>

	<div class="pic" role="img" aria-label={label} bind:this={pic}>
		<div class="side" aria-hidden="true">{@render panel('laptop')}</div>

		<div class="hop" aria-hidden="true">
			<span class="wire"></span>
			<span class="chip" class:fire>repose run</span>
			<span class="wire arrow"></span>
			<span class="ready m-later">Ready in 0.6s</span>
		</div>

		<div class="side" aria-hidden="true">{@render panel('machine')}</div>

		<div class="fly" aria-hidden="true" bind:this={fly}></div>
	</div>
</div>

<style>
	.oc h2 {
		text-wrap: balance;
	}
	.pic {
		--accent: var(--color-blue-600);
		--ink: var(--color-zinc-800);
		--dim: var(--color-zinc-500);
		--faint: var(--color-zinc-400);
		--stop: var(--color-red-600);
		position: relative;
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		margin-top: 36px;
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
		color: var(--ink);
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
		color: var(--ink);
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
		color: var(--ink);
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
		background: var(--accent);
		z-index: 1;
	}
	.graph .commit:not(:last-child)::after {
		content: '';
		position: absolute;
		left: 24px;
		top: 13px;
		height: 26px;
		border-left: 1.5px solid var(--accent);
	}
	.commit.base .node {
		background: var(--surface);
		border: 1.5px solid var(--faint);
	}
	.msg {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.base .msg {
		color: var(--dim);
	}
	.hash,
	.ref {
		flex: none;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--dim);
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
		color: var(--dim);
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
		color: var(--dim);
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
		color: var(--dim);
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
		color: var(--dim);
	}

	/* node_modules: the laptop's is struck and stays; the machine's is its own. */
	.nm .fi,
	.nm .fname {
		color: var(--faint);
	}
	.m-nm .fname {
		color: var(--dim);
	}
	.l-stop .fname {
		position: relative;
	}
	.strike {
		position: absolute;
		left: -2px;
		right: -2px;
		top: calc(50% - 2px);
		border-top: 1.5px solid var(--stop);
		transform-origin: left center;
	}
	.stop {
		flex: none;
		margin-left: auto;
		color: var(--stop);
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
		border-left: 1.5px solid var(--accent);
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
		border-top: 6px solid var(--accent);
		border-bottom: 0;
	}
	.chip {
		padding: 3px 9px;
		border: 1px solid var(--accent);
		border-radius: 3px;
		background: var(--surface);
		font-family: var(--font-mono);
		font-size: 13px;
		font-weight: 600;
		color: var(--accent);
		white-space: nowrap;
		transition:
			background-color 0.18s,
			color 0.18s;
	}
	.chip.fire {
		background: var(--accent);
		color: var(--surface);
	}
	.ready {
		position: absolute;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--color-zinc-600);
		display: inline-flex;
		align-items: center;
		gap: 6px;
		top: 50%;
		left: calc(50% + 62px);
		transform: translateY(-50%);
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
		color: var(--ink);
	}
	.fly :global(.clone) {
		position: absolute;
		margin: 0;
		list-style: none;
		background: color-mix(in oklab, var(--accent) 9%, var(--surface));
		box-shadow: inset 2px 0 0 var(--accent);
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
			border-top: 1.5px solid var(--accent);
		}
		.wire.arrow::after {
			left: auto;
			right: -1px;
			bottom: auto;
			top: -6.25px;
			border: 5px solid transparent;
			border-left: 6px solid var(--accent);
			border-right: 0;
		}
		.ready {
			top: calc(50% + 24px);
			left: 50%;
			transform: translateX(-50%);
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
		.pic {
			--accent: var(--color-blue-400);
			--ink: var(--color-zinc-200);
			--dim: var(--color-zinc-400);
			--faint: var(--color-zinc-600);
			--stop: var(--color-red-400);
		}
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
