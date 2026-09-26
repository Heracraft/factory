<!--
  Break it and roll it back, drawn in the working-state section's style
  (OneCommand.svelte): light panels, hairline borders, rows, the blue accent
  for what moves. Data from the real run on the owner's project wira
  (2026-09-25, captures in the landing scratch): four of the checkout's
  top-level entries (`ls ~/wira`, cropped to README.md, package.json and the
  two folders the agent deleted); `git status -sb` after the damage showed
  public/ files as ` D`; the snapshot restored was the manual one taken at
  21:22 (`repose snapshots list --project wira`), after which `ls` showed
  public and src again.

  The loop, with anime.js: a snapshot is taken (the rows copy into the
  snapshot panel, stamped 21:22); src/ and public/ are deleted (red, struck,
  D, then collapse); the snapshot puts them back (blue copies travel home,
  a green tick lands); rest; again. Under prefers-reduced-motion, or before
  anime.js loads, the restored state is shown still.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	type Row = { name: string; dir?: boolean; gone?: boolean };
	const rows: Row[] = [
		{ name: 'public/', dir: true, gone: true },
		{ name: 'src/', dir: true, gone: true },
		{ name: 'README.md' },
		{ name: 'package.json' }
	];

	const label =
		'Your cloud machine holds your repo: public, src, README.md and package.json. A snapshot of it is taken at 21:22. ' +
		'The agent deletes src and public; restoring the 21:22 snapshot brings both back.';

	let pic: HTMLDivElement;
	let fly: HTMLDivElement;

	onMount(() => {
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

		let dead = false;
		let visible = false;
		let tl: { pause(): unknown; play(): unknown } | null = null;
		let io: IntersectionObserver | undefined;

		const q = (s: string) => Array.from(pic.querySelectorAll<HTMLElement>(s));

		// Before anything: the machine is whole, no snapshot yet.
		function pre(utils: typeof import('animejs').utils) {
			utils.set(q('.s-row, .s-time, .arr'), { opacity: 0 });
			utils.set(q('.m-row'), { opacity: 1, height: 26 });
			utils.set(q('.strike'), { scaleX: 0 });
			utils.set(q('.del'), { opacity: 0, scale: 0.6 });
			utils.set(q('.tick'), { opacity: 0, scale: 0.6 });
			for (const el of q('.hit, .lit, .fire')) el.classList.remove('hit', 'lit', 'fire');
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
				// .fly is an empty layer Svelte never renders into; only
				// throwaway copies of rows live in it.
				// eslint-disable-next-line svelte/no-dom-manipulating
				fly.replaceChildren();
				pre(utils);

				const base = pic.getBoundingClientRect();
				const mRows = q('.m-row');
				const sRows = q('.s-row');
				const cls = (els: HTMLElement[], c: string, on: boolean) => () => {
					for (const el of els) el.classList.toggle(c, on);
				};

				function clone(from: HTMLElement, to: HTMLElement) {
					const a = from.getBoundingClientRect();
					const b = to.getBoundingClientRect();
					const c = from.cloneNode(true) as HTMLElement;
					c.classList.add('clone');
					c.classList.remove('m-row', 's-row');
					Object.assign(c.style, {
						left: `${a.left - base.left}px`,
						top: `${a.top - base.top}px`,
						width: `${a.width}px`,
						height: `${a.height}px`,
						opacity: '0'
					});
					// eslint-disable-next-line svelte/no-dom-manipulating
					fly.appendChild(c);
					return { c, dx: b.left - a.left, dy: b.top - a.top, w: b.width, to };
				}

				// Side by side the copies travel sideways; stacked (never at
				// this card's widths) they would travel down. Either way top
				// to bottom.
				const shots = mRows.map((m, i) => clone(m, sRows[i]));
				const backs = mRows
					.map((m, i) => (m.classList.contains('gone') ? clone(sRows[i], m) : null))
					.filter((x) => x !== null);

				const gone = q('.m-row.gone');
				const T = {
					snap: 500,
					del: 2800,
					fold: 4000,
					back: 5300
				};
				const dur = 850;
				const step = 90;
				const t = createTimeline({ autoplay: false, onComplete: () => cycle() });

				// 1. the snapshot: the camera fires, the rows copy across, 21:22
				t.call(cls(q('.cam, .win.snap'), 'fire', true), T.snap)
					.add(q('.s-time'), { opacity: [0, 1], duration: 300 }, T.snap + 100)
					.add(q('.take'), { opacity: [0, 1], duration: 250 }, T.snap)
					.add(q('.take'), { opacity: 0, duration: 400 }, T.snap + 150 + step * 3 + dur + 200);
				shots.forEach(({ c, dx, dy, w, to }, i) => {
					const s = T.snap + 150 + step * i;
					t.set(c, { opacity: 1 }, s)
						.add(c, { x: dx, y: dy, width: w, duration: dur, ease: 'inOutCubic' }, s)
						.set(to, { opacity: 1 }, s + dur)
						.set(c, { opacity: 0 }, s + dur + 16);
				});
				t.call(cls(q('.cam, .win.snap'), 'fire', false), T.snap + 150 + step * 3 + dur + 200);

				// 2. the damage: red, struck, D; then the rows fold away
				t.call(cls(gone, 'hit', true), T.del)
					.add(
						q('.gone .strike'),
						{ scaleX: [0, 1], duration: 380, delay: stagger(160), ease: 'outCubic' },
						T.del + 120
					)
					.add(
						q('.gone .del'),
						{
							opacity: [0, 1],
							scale: [0.6, 1],
							duration: 280,
							delay: stagger(160),
							ease: 'outBack'
						},
						T.del + 300
					)
					.add(gone, { opacity: 0, duration: 220, ease: 'inQuad' }, T.fold)
					.add(gone, { height: 0, duration: 480, ease: 'inOutQuad' }, T.fold + 120);

				// 3. the restore: the snapshot's copies travel home, the rows open under them
				t.call(cls(q('.s-row.gone'), 'lit', true), T.back - 300)
					.add(q('.give'), { opacity: [0, 1], duration: 250 }, T.back - 300)
					.call(cls(gone, 'hit', false), T.back)
					.set(q('.gone .strike'), { scaleX: 0 }, T.back)
					.set(q('.gone .del'), { opacity: 0 }, T.back)
					.add(gone, { height: [0, 26], duration: dur, ease: 'inOutCubic' }, T.back);
				backs.forEach(({ c, dx, dy, w, to }, i) => {
					const s = T.back + step * i;
					t.set(c, { opacity: 1 }, s)
						.add(c, { x: dx, y: dy, width: w, duration: dur, ease: 'inOutCubic' }, s)
						.set(to, { opacity: 1 }, s + dur)
						.set(c, { opacity: 0 }, s + dur + 16)
						.add(
							to.querySelector('.tick')!,
							{ opacity: [0, 1], scale: [0.6, 1], duration: 300, ease: 'outBack' },
							s + dur + 40
						);
				});
				const landed = T.back + step * (backs.length - 1) + dur;
				t
					// rest on the restored machine, then clear the snapshot and go again
					.add(
						q('.s-row, .s-time, .tick, .give'),
						{ opacity: 0, duration: 450, ease: 'inQuad' },
						landed + 5200
					)
					.add({ duration: 500 }, landed + 5650);

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
				{ threshold: 0.6 }
			);
			io.observe(pic);
		});

		return () => {
			dead = true;
			io?.disconnect();
			tl?.pause();
		};
	});
</script>

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

{#snippet row(r: Row, side: 'm' | 's')}
	<li class="fr {side}-row" class:gone={r.gone} class:lit={side === 's' && r.gone}>
		{#if r.dir}{@render folderIcon()}{:else}{@render fileIcon()}{/if}
		<span class="nm"
			>{r.name}{#if side === 'm' && r.gone}<i class="strike"></i>{/if}</span
		>
		{#if side === 'm' && r.gone}
			<span class="mark">
				<span class="del">D</span>
				<svg class="tick" viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"
					><path
						d="M2.5 6.2l2.3 2.3 4.7-5"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
					/></svg
				>
			</span>
		{/if}
	</li>
{/snippet}

<div class="min-w-0">
	<div
		class="h-60 overflow-hidden rounded-xs border border-[var(--rule)] bg-[var(--sunken)]"
		role="img"
		aria-label={label}
	>
		<div class="pic" bind:this={pic} aria-hidden="true">
			<div class="win machine">
				<div class="title">
					<svg viewBox="0 0 24 24" width="17" height="17" aria-hidden="true" class="tm">
						<path
							d="M7 18.5h10.5a4 4 0 0 0 .6-7.96A5.5 5.5 0 0 0 7.4 9.1 4.7 4.7 0 0 0 7 18.5z"
							fill="none"
							stroke="currentColor"
							stroke-width="1.5"
							stroke-linejoin="round"
						/>
					</svg>
					<span class="who">your cloud machine</span><i class="dot"></i>
				</div>
				<ul class="rows">
					{#each rows as r (r.name)}{@render row(r, 'm')}{/each}
				</ul>
			</div>

			<div class="hop">
				<svg class="arr take" viewBox="0 0 20 10" aria-hidden="true"
					><path
						d="M1 5h16M13 1.5L17.5 5 13 8.5"
						fill="none"
						stroke="currentColor"
						stroke-width="1.5"
					/></svg
				>
				<svg class="arr give" viewBox="0 0 20 10" aria-hidden="true"
					><path
						d="M19 5H3M7 1.5L2.5 5 7 8.5"
						fill="none"
						stroke="currentColor"
						stroke-width="1.5"
					/></svg
				>
			</div>

			<div class="win snap">
				<div class="title">
					<svg viewBox="0 0 20 16" width="17" height="14" aria-hidden="true" class="tm cam">
						<path
							d="M1.5 4.5h4l1.6-2.5h5.8l1.6 2.5h4v9.5h-17z"
							fill="none"
							stroke="currentColor"
							stroke-width="1.4"
							stroke-linejoin="round"
						/>
						<circle cx="10" cy="9" r="3" fill="none" stroke="currentColor" stroke-width="1.4" />
					</svg>
					<span class="s-time">21:22</span>
				</div>
				<ul class="rows">
					{#each rows as r (r.name)}{@render row(r, 's')}{/each}
				</ul>
			</div>

			<div class="fly" bind:this={fly}></div>
		</div>
	</div>
	<h3 class="mt-5 text-lg font-semibold">Break it and roll it back</h3>
	<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
		The disk is snapshotted every night, at every stop and whenever you ask, and a destroyed project
		comes back within 30 days.
	</p>
</div>

<style>
	.pic {
		--accent: var(--color-blue-600);
		--ink: var(--color-zinc-800);
		--dim: var(--color-zinc-500);
		--faint: var(--color-zinc-400);
		--stop: var(--color-red-600);
		--ok: var(--color-emerald-600);
		position: relative;
		height: 100%;
		display: grid;
		grid-template-columns: minmax(0, 1.2fr) 32px minmax(0, 1fr);
		align-items: center;
		padding: 0 24px;
		font-size: 13px;
		color: var(--ink);
	}

	.win {
		display: flex;
		flex-direction: column;
		/* fixed, so folding rows away leaves the panel still */
		height: 152px;
		border: 1px solid var(--rule-strong);
		border-radius: 3px;
		background: var(--surface);
		overflow: hidden;
	}
	.win {
		transition: border-color 0.3s;
	}
	.win.fire {
		border-color: var(--accent);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 34px;
		flex: none;
		padding: 0 12px;
		border-bottom: 1px solid var(--rule);
		background: var(--sunken);
	}
	.tm {
		flex: none;
		color: var(--ink);
	}
	.cam {
		transition: color 0.2s;
	}
	.cam.fire {
		color: var(--accent);
	}
	.who {
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.dot {
		flex: none;
		width: 7px;
		height: 7px;
		margin-left: auto;
		border-radius: 50%;
		background: var(--color-emerald-500);
	}
	.s-time {
		font-family: var(--font-mono);
		font-size: 12.5px;
		font-weight: 600;
	}

	.rows {
		margin: 0;
		padding: 7px 0;
		list-style: none;
	}
	.fr {
		position: relative;
		display: flex;
		align-items: center;
		gap: 7px;
		height: 26px;
		padding: 0 12px 0 14px;
		white-space: nowrap;
		overflow: hidden;
	}
	.fi {
		flex: none;
		color: var(--dim);
	}
	.nm {
		position: relative;
		flex: none;
		font-family: var(--font-mono);
		font-size: 12.5px;
	}
	.snap .nm {
		color: var(--dim);
	}
	.fr {
		transition: background-color 0.25s;
	}
	.nm {
		transition: color 0.25s;
	}
	.fr.hit {
		background: color-mix(in oklab, var(--stop) 9%, var(--surface));
	}
	.hit .nm {
		color: var(--stop);
	}
	.fr.lit {
		background: color-mix(in oklab, var(--accent) 9%, var(--surface));
		box-shadow: inset 2px 0 0 var(--accent);
	}
	.lit .nm {
		color: var(--ink);
	}
	.strike {
		position: absolute;
		left: -2px;
		right: -2px;
		top: calc(50% - 1px);
		border-top: 1.5px solid var(--stop);
		transform: scaleX(0);
		transform-origin: left center;
	}
	.mark {
		position: relative;
		flex: none;
		width: 12px;
		height: 14px;
		margin-left: auto;
	}
	.del,
	.tick {
		position: absolute;
		inset: 0;
		margin: auto;
	}
	.del {
		opacity: 0;
		text-align: center;
		font-family: var(--font-mono);
		font-size: 12px;
		font-weight: 600;
		line-height: 14px;
		color: var(--stop);
	}
	.tick {
		color: var(--ok);
	}

	/* Between the panels: which way the copy goes. */
	.hop {
		position: relative;
		height: 10px;
	}
	.arr {
		position: absolute;
		inset: 0;
		margin: auto;
		width: 20px;
		height: 10px;
	}
	.take {
		opacity: 0;
		color: var(--dim);
	}
	.give {
		color: var(--accent);
	}
	@media (max-width: 480px) {
		.arr {
			width: 16px;
		}
	}

	/* Rows in flight: copies, tinted like the working-state section's. */
	.fly {
		position: absolute;
		inset: 0;
		z-index: 1;
		pointer-events: none;
	}
	.fly :global(.clone) {
		position: absolute;
		margin: 0;
		list-style: none;
		background: color-mix(in oklab, var(--accent) 9%, var(--surface));
		box-shadow: inset 2px 0 0 var(--accent);
		will-change: transform;
	}
	.fly :global(.clone .nm) {
		color: var(--ink);
	}
	.fly :global(.clone .mark) {
		display: none;
	}

	@media (max-width: 480px) {
		.pic {
			grid-template-columns: minmax(0, 1.35fr) 20px minmax(0, 1fr);
			padding: 0 14px;
			font-size: 12px;
		}
		.title {
			padding: 0 9px;
			gap: 6px;
			font-size: 11.5px;
		}
		.fr {
			gap: 6px;
			padding: 0 9px 0 10px;
		}
		.nm,
		.s-time {
			font-size: 11.5px;
		}
	}
	@media (prefers-color-scheme: dark) {
		.pic {
			--accent: var(--color-blue-400);
			--ink: var(--color-zinc-200);
			--dim: var(--color-zinc-400);
			--faint: var(--color-zinc-600);
			--stop: var(--color-red-400);
			--ok: var(--color-emerald-400);
		}
	}
</style>
