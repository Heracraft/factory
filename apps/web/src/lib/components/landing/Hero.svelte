<!--
  The hero: the whole pitch as one moving picture, in the style of "Your
  working state, in one command" (OneCommand.svelte): two panels with
  hairline borders, rows and chips, blue for what moves, anime.js.

  Left, your laptop: your repo and a few private things (SSH keys, a
  passwords file, a cat photo, a tax return). `repose run`: the repo folder
  travels into your cloud machine and opens there; the private things stay.
  A snapshot is taken: the machine flashes and a miniature of it shrinks onto
  the rail under it. The agent (Claude Code's mark, orange) turns red and
  strikes src/ and public/; it reaches out to the internet and pulls in a
  "malicious skill", which runs at your laptop's private things and stops at
  the machine's wall. The miniature grows back over the machine and the
  folders are back; the agent is orange again.

  What is named on the machine is from the real run on the owner's project
  wira (2026-09-25, the landing work's wreck/ captures): `ls ~/wira` after the
  restore (06-after-restore.txt); Claude Code in bypass permissions mode ran
  `rm -rf ~/wira/src ~/wira/public` (03-damage.txt shows them gone); the
  restored snapshot is the manual one taken at 21:22 (04-restore.ansi). The
  laptop's private items are illustration, and "malicious skill" is not a
  real package. What reaches what is /docs/secrets "What an agent on the
  machine can reach": the machine's own contents and the internet are
  reachable from it, the laptop is not; the picture shows the attack failing
  against the laptop only, never the machine's own files as safe.

  Without JS, and under prefers-reduced-motion, one still frame tells the
  whole story: the repo on both sides, the snapshot on the rail with an
  arrow back up, src/ and public/ struck, the agent red, the skill stopped at
  the wall. Playback pauses off screen.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { agentMarks } from '$lib/components/illustrations/marks';

	const agent = agentMarks[0];

	const repo = 'wira';
	// `ls ~/wira`, cut to four; x: deleted by the agent.
	const kids: { n: string; dir?: boolean; x?: boolean }[] = [
		{ n: 'src/', dir: true, x: true },
		{ n: 'public/', dir: true, x: true },
		{ n: 'package.json' },
		{ n: 'README.md' }
	];
	type Priv = { n: string; icon: 'key' | 'sheet' | 'image' | 'doc' };
	const privs: Priv[] = [
		{ n: '.ssh/', icon: 'key' },
		{ n: 'passwords.csv', icon: 'sheet' },
		{ n: 'cat.jpg', icon: 'image' },
		{ n: 'taxes-2025.pdf', icon: 'doc' }
	];

	const label =
		'Your laptop holds your repo and your private things: SSH keys, passwords.csv, cat.jpg and a tax return. ' +
		'repose run copies only the repo to your cloud machine, and a snapshot of the machine is taken. ' +
		'The agent on the machine turns rogue and deletes src and public, then pulls in a malicious skill from the internet ' +
		'that goes for your private things and is stopped at the machine’s wall. The snapshot is restored and the folders are back.';

	let pic: HTMLDivElement;
	let fly: HTMLDivElement;
	let fire = $state(false);

	type Geo = {
		beams: string[];
		reach: string;
		back: string;
		backHead: string;
		w: number;
		h: number;
	};
	let geo: Geo | null = $state(null);
	// The snapshot miniature: the machine's size and the scale it shrinks by.
	let mini = $state({ w: 360, h: 236, s: 0.3 });

	function measure() {
		const box = pic.getBoundingClientRect();
		const r = (el: Element) => {
			const b = el.getBoundingClientRect();
			return {
				l: b.left - box.left,
				r: b.right - box.left,
				t: b.top - box.top,
				b: b.bottom - box.top,
				cx: b.left - box.left + b.width / 2,
				cy: b.top - box.top + b.height / 2,
				w: b.width,
				h: b.height
			};
		};
		const win = pic.querySelector('.mwin .win')!;
		const W = r(win);
		const wide = window.matchMedia('(min-width: 768px)').matches;
		mini = { w: W.w, h: W.h, s: (wide ? 128 : 100) / W.w };

		const ag = r(pic.querySelector('.mwin .agent')!);
		const beams = Array.from(pic.querySelectorAll('.mwin .kid.x .fname')).map((el) => {
			const f = r(el);
			const x0 = ag.l - 4,
				y0 = ag.cy;
			const x1 = f.r + 10,
				y1 = f.cy;
			return `M${x0} ${y0}C${x0 - 30} ${y0} ${x1 + 30} ${y1} ${x1} ${y1}`;
		});
		const chip = r(pic.querySelector('.net .skill')!);
		const reach = wide ? `M${W.r} ${chip.cy}H${chip.l - 3}` : `M${chip.cx} ${W.b}V${chip.t - 3}`;
		const th = r(pic.querySelector('.rail .slot')!);
		const back = `M${th.cx} ${th.t - 2}V${W.b + 2}`;
		const backHead = `M${th.cx - 4} ${W.b + 7}L${th.cx} ${W.b + 2}L${th.cx + 4} ${W.b + 7}`;
		geo = { beams, reach, back, backHead, w: box.width, h: box.height };
	}

	onMount(() => {
		measure();
		let dead = false;
		let visible = false;
		let tl: { pause(): unknown; play(): unknown; revert(): unknown } | null = null;
		let io: IntersectionObserver | undefined;
		let raf = 0;
		const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		const onResize = () => {
			cancelAnimationFrame(raf);
			raf = requestAnimationFrame(() => {
				if (still || !tl) return measure();
				tl.pause();
				tl = null;
				fresh = true;
				if (visible) go?.();
			});
		};
		window.addEventListener('resize', onResize);
		let go: (() => void) | undefined;
		let fresh = true;

		if (!still) {
			const q = (s: string) => Array.from(pic.querySelectorAll<HTMLElement>(s));
			import('animejs').then(({ createTimeline, utils, stagger }) => {
				if (dead) return;
				pic.classList.add('live');

				// Before the run: the machine is empty, nothing is struck, the
				// agent is itself, no snapshot yet.
				function pre() {
					utils.set(q('.m-in, .m-kid, .m-late, .mwin .agent, .thumb, .t-time, .back'), {
						opacity: 0,
						x: 0,
						y: 0,
						scale: 1
					});
					utils.set(q('.mwin .strike'), { scaleX: 0 });
					utils.set(q('.mwin .xbg, .mwin .red, .wallhit, .stopper, .mwin .shutter'), {
						opacity: 0
					});
					utils.set(q('.mwin .kid.x .fname, .mwin .kid.x .fi'), { opacity: 1 });
					utils.set(q('.skill-in'), { opacity: 0, x: 0, y: 0 });
					utils.set(q('.net .skill'), { opacity: 1 });
					utils.set(q('.priv .tg'), { opacity: 0 });
					utils.set(q('.lines path'), { strokeDashoffset: 1, opacity: 1 });
					fire = false;
					pic.classList.remove('wrecked');
				}

				function cycle() {
					if (dead) return;
					if (!visible) {
						tl = null;
						return;
					}
					// .fly is an empty layer Svelte never renders into; only a
					// throwaway copy of the laptop's repo row lives in it.
					// eslint-disable-next-line svelte/no-dom-manipulating
					fly.replaceChildren();
					utils.set(q('.thumb, .back, .skill-in'), { x: 0, y: 0, scale: 1 });
					pic.classList.remove('live');
					measure();
					pic.classList.add('live');
					pre();

					const box = pic.getBoundingClientRect();
					const rel = (el: Element) => {
						const b = el.getBoundingClientRect();
						return { l: b.left - box.left, t: b.top - box.top, w: b.width, h: b.height };
					};
					// The repo row's copy, flying from the laptop to the machine.
					const from = pic.querySelector<HTMLElement>('[data-from]')!;
					const to = pic.querySelector<HTMLElement>('[data-to]')!;
					const a = rel(from);
					const b = rel(to);
					const c = from.cloneNode(true) as HTMLElement;
					c.removeAttribute('data-from');
					c.classList.add('clone');
					Object.assign(c.style, {
						left: `${a.l}px`,
						top: `${a.t}px`,
						width: `${a.w}px`,
						opacity: '0'
					});
					// eslint-disable-next-line svelte/no-dom-manipulating
					fly.appendChild(c);

					// The miniature: from covering the machine to its slot.
					const win = rel(pic.querySelector('.mwin .win')!);
					const slot = rel(pic.querySelector('.rail .slot')!);
					const k = win.w / slot.w;
					const cover = { x: win.l - slot.l, y: win.t - slot.t, scale: k };

					// The skill: from the internet, to the agent, to the wall.
					const home = rel(pic.querySelector('.skill-in')!);
					const src = rel(pic.querySelector('.net .skill')!);
					const ag = rel(pic.querySelector('.mwin .agent')!);
					const at = { x: src.l - home.l, y: src.t - home.t };
					const dock = { x: ag.l - 10 - home.w - home.l, y: 0 };

					const T = {
						run: 600,
						fly: 800,
						open: 1750,
						agent: 2300,
						shot: 3300,
						rogue: 5000,
						strike: 5600,
						reach: 6900,
						pull: 7300,
						lunge: 8700,
						hit: 9400,
						restore: 10500,
						back: 11300,
						out: 15000
					};
					const t = createTimeline({ autoplay: false, onComplete: () => cycle() })
						// repose run: the repo row travels, the machine opens it.
						.call(() => (fire = true), T.run)
						.call(() => (fire = false), T.run + 520)
						.set(c, { opacity: 1 }, T.fly)
						.add(c, { x: b.l - a.l, y: b.t - a.t, duration: 900, ease: 'inOutCubic' }, T.fly)
						.set(q('.m-in'), { opacity: 1 }, T.fly + 900)
						.set(c, { opacity: 0 }, T.fly + 916)
						.add(
							q('.m-kid'),
							{ opacity: [0, 1], y: [-6, 0], duration: 320, delay: stagger(90), ease: 'outCubic' },
							T.open
						)
						.add(q('.m-late'), { opacity: [0, 1], duration: 300 }, T.open + 200)
						.add(
							q('.mwin .agent'),
							{ opacity: [0, 1], scale: [0.6, 1], duration: 380, ease: 'outBack' },
							T.agent
						)
						// Snapshot: the shutter flashes and a miniature of the machine
						// shrinks onto the rail.
						.add(q('.rail .cam'), { scale: [1, 1.25, 1], duration: 420, ease: 'outQuad' }, T.shot)
						.add(
							q('.mwin .shutter'),
							{ opacity: [0, 0.9, 0], duration: 460, ease: 'outQuad' },
							T.shot
						)
						.set(q('.thumb'), { opacity: 1, ...cover }, T.shot + 120)
						.add(
							q('.thumb'),
							{ x: 0, y: 0, scale: 1, duration: 850, ease: 'inOutCubic' },
							T.shot + 220
						)
						.add(q('.t-time'), { opacity: [0, 1], duration: 300 }, T.shot + 1000)
						// The agent goes rogue and deletes src/ and public/.
						.add(q('.mwin .agent .red'), { opacity: [0, 1], duration: 350 }, T.rogue)
						.add(
							q('.mwin .agent'),
							{ x: [0, -2, 2, -1, 0], duration: 360, ease: 'linear' },
							T.rogue + 200
						)
						.add(
							q('.lines .beam'),
							{ strokeDashoffset: [1, 0], duration: 420, delay: stagger(120), ease: 'outCubic' },
							T.rogue + 500
						)
						.call(() => pic.classList.add('wrecked'), T.strike)
						.add(
							q('.mwin .kid.x .xbg'),
							{ opacity: [0, 1], duration: 250, delay: stagger(150) },
							T.strike
						)
						.add(
							q('.mwin .kid.x .strike'),
							{ scaleX: [0, 1], duration: 380, delay: stagger(150), ease: 'outCubic' },
							T.strike + 80
						)
						.add(
							q('.mwin .kid.x .fname, .mwin .kid.x .fi'),
							{ opacity: [1, 0.45], duration: 400 },
							T.strike + 500
						)
						.add(q('.lines .beam'), { opacity: [1, 0], duration: 350 }, T.strike + 900)
						// It reaches out and pulls in the malicious skill.
						.add(
							q('.lines .reach'),
							{ strokeDashoffset: [1, 0], duration: 380, ease: 'outCubic' },
							T.reach
						)
						.set(q('.net .skill'), { opacity: 0.25 }, T.pull)
						.set(q('.skill-in'), { opacity: 1, ...at }, T.pull)
						.add(q('.skill-in'), { ...dock, duration: 1000, ease: 'inOutCubic' }, T.pull)
						.add(q('.lines .reach'), { opacity: [1, 0], duration: 300 }, T.pull + 1000)
						// It goes for the laptop's private things and stops at the wall.
						.add(q('.priv .tg'), { opacity: [0, 1], duration: 300, delay: stagger(60) }, T.lunge)
						.add(q('.skill-in'), { x: 0, y: 0, duration: 700, ease: 'inQuad' }, T.lunge)
						.add(q('.wallhit'), { opacity: [0, 1], duration: 120 }, T.hit)
						.add(
							q('.stopper'),
							{ opacity: [0, 1], scale: [0.6, 1], duration: 300, ease: 'outBack' },
							T.hit
						)
						.add(q('.skill-in'), { x: [0, 7, 0], duration: 380, ease: 'outQuad' }, T.hit)
						.add(q('.priv .tg'), { opacity: 0, duration: 500 }, T.hit + 700)
						// The snapshot comes back over the machine.
						.add(q('.lines .ret'), { strokeDashoffset: [1, 0], duration: 300 }, T.restore)
						.set(q('.back'), { opacity: 1 }, T.restore + 250)
						.add(q('.back'), { ...cover, duration: 800, ease: 'inOutCubic' }, T.restore + 280)
						.add(q('.lines .ret'), { opacity: [1, 0], duration: 250 }, T.back)
						.call(() => pic.classList.remove('wrecked'), T.back + 100)
						.set(q('.mwin .strike'), { scaleX: 0 }, T.back + 100)
						.set(
							q('.mwin .xbg, .mwin .agent .red, .wallhit, .stopper, .skill-in'),
							{ opacity: 0 },
							T.back + 100
						)
						.set(q('.mwin .kid.x .fname, .mwin .kid.x .fi'), { opacity: 1 }, T.back + 100)
						.add(q('.back'), { opacity: [1, 0], duration: 500, ease: 'inQuad' }, T.back + 150)
						.add(q('.net .skill'), { opacity: 1, duration: 400 }, T.back + 400)
						// Rest on the restored machine, then clear it and go again.
						.add(
							q('.m-in, .m-kid, .m-late, .mwin .agent, .thumb, .t-time'),
							{ opacity: 0, duration: 450, ease: 'inQuad' },
							T.out
						)
						.add({ duration: 500 }, T.out + 450);

					tl = t;
					fresh = false;
					t.play();
				}
				go = cycle;

				io = new IntersectionObserver(
					([e]) => {
						visible = e.isIntersecting;
						if (visible) {
							if (tl && !fresh) tl.play();
							else cycle();
						} else {
							tl?.pause();
						}
					},
					{ threshold: 0.35 }
				);
				io.observe(pic);
			});
		}

		return () => {
			dead = true;
			io?.disconnect();
			tl?.pause();
			window.removeEventListener('resize', onResize);
			cancelAnimationFrame(raf);
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

{#snippet repoIcon()}
	<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="fi">
		<g fill="none" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round">
			<path d="M1.5 3.5h4.3l1.4 1.6h7.3v8.4h-13z" />
			<circle cx="6" cy="10.8" r="1.1" />
			<circle cx="10.5" cy="8.4" r="1.1" />
			<path d="M6 9.7V7.6M10.5 9.5c0 1.2-4.5 0.4-4.5 1.3" />
		</g>
	</svg>
{/snippet}

{#snippet privIcon(kind: Priv['icon'])}
	<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" class="fi">
		<g fill="none" stroke="currentColor" stroke-width="1.1" stroke-linejoin="round">
			{#if kind === 'key'}
				<circle cx="5" cy="8" r="2.8" />
				<path d="M7.8 8h6.2M12 8v2.3M10 8v1.7" stroke-linecap="round" />
			{:else if kind === 'sheet'}
				<rect x="2.5" y="2.5" width="11" height="11" />
				<path d="M2.5 6.2h11M2.5 9.8h11M6.5 2.5v11" />
			{:else if kind === 'image'}
				<rect x="2" y="3" width="12" height="10" />
				<path d="M2 11l3.5-3.2 2.7 2.4 2-1.7L14 11.6" />
				<circle cx="10.6" cy="5.9" r="1" />
			{:else}
				<path d="M4 1.5h5.2L12.5 4.8v9.7H4z M9 1.5v3.5h3.5" />
				<path d="M6 8.5h4.5M6 11h4.5" />
			{/if}
		</g>
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

{#snippet globeMark()}
	<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true" class="globe">
		<g fill="none" stroke="currentColor" stroke-width="1.4">
			<circle cx="12" cy="12" r="9" />
			<ellipse cx="12" cy="12" rx="4" ry="9" />
			<path d="M3.5 9h17M3.5 15h17" />
		</g>
	</svg>
{/snippet}

{#snippet stopIcon()}
	<svg viewBox="0 0 16 16" width="18" height="18" aria-hidden="true">
		<circle cx="8" cy="8" r="6.5" fill="var(--surface)" />
		<g fill="none" stroke="currentColor" stroke-width="1.5">
			<circle cx="8" cy="8" r="6" />
			<path d="M3.8 12.2l8.4-8.4" />
		</g>
	</svg>
{/snippet}

{#snippet skillChip(cls: string)}
	<span class="skill {cls}">
		<svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
			<g fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">
				<path d="M8 1.8l5.5 3v6.4L8 14.2l-5.5-3V4.8z" />
				<path d="M2.5 4.8L8 7.8l5.5-3M8 7.8v6.4" />
			</g>
		</svg>
		malicious skill
	</span>
{/snippet}

{#snippet machineWin(live: boolean)}
	<div class="win">
		<div class="title">
			{@render cloudMark()}<span class="who">your cloud machine</span><i
				class="dot"
				class:m-late={live}
			></i>
		</div>
		<ul class="rows">
			<li class="fr head" class:m-in={live} data-to={live ? 'repo' : undefined}>
				<svg viewBox="0 0 10 10" width="9" height="9" aria-hidden="true" class="chev">
					<path d="M2 3.5l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.3" />
				</svg>
				{@render repoIcon()}<span class="fname">{repo}/</span>
			</li>
			{#each kids as k (k.n)}
				<li class="fr kid" class:x={k.x} class:m-kid={live}>
					{#if k.x}<i class="xbg"></i>{/if}
					{#if k.dir}{@render folderIcon()}{:else}{@render fileIcon()}{/if}
					<span class="fname"
						>{k.n}{#if k.x}<i class="strike"></i>{/if}</span
					>
				</li>
			{/each}
		</ul>
		<div class="lane">
			{#if live}
				<span class="wallhit"></span>
				<span class="stopper">{@render stopIcon()}</span>
				{@render skillChip('skill-in')}
			{/if}
			<span class="agent">
				<i class="halo red"></i>
				<svg viewBox="0 0 24 24" width="30" height="30" aria-hidden="true">
					{#each agent.paths as d (d)}
						<path {d} fill-rule="evenodd" class="orange" />
					{/each}
				</svg>
				<svg viewBox="0 0 24 24" width="30" height="30" aria-hidden="true" class="red">
					{#each agent.paths as d (d)}
						<path {d} fill-rule="evenodd" />
					{/each}
				</svg>
			</span>
		</div>
		{#if live}<i class="shutter"></i>{/if}
	</div>
{/snippet}

<div class="hero-pic" role="img" aria-label={label} bind:this={pic}>
	<div class="side laptop" aria-hidden="true">
		<div class="win">
			<div class="title">{@render laptopMark()}<span class="who">your laptop</span></div>
			<ul class="rows">
				<li class="fr head repo" data-from="repo">
					<svg viewBox="0 0 10 10" width="9" height="9" aria-hidden="true" class="chev side-chev">
						<path d="M3.5 2l3 3-3 3" fill="none" stroke="currentColor" stroke-width="1.3" />
					</svg>
					{@render repoIcon()}<span class="fname">{repo}/</span>
				</li>
				{#each privs as p (p.n)}
					<li class="fr priv">
						<i class="tg"></i>
						{@render privIcon(p.icon)}<span class="fname">{p.n}</span>
					</li>
				{/each}
			</ul>
		</div>
	</div>

	<div class="hop" aria-hidden="true">
		<span class="wire"></span>
		<span class="chip" class:fire>repose run</span>
		<span class="wire arrow"></span>
	</div>

	<div class="machine" aria-hidden="true">
		<div class="mwin">{@render machineWin(true)}</div>

		<div class="net">
			{@render globeMark()}
			{@render skillChip('')}
		</div>

		<div class="rail">
			<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" class="cam">
				<g fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">
					<path d="M1.8 5h3l1.2-1.8h4L11.2 5h3v8h-12.4z" />
					<circle cx="8" cy="8.8" r="2.4" />
				</g>
			</svg>
			<span class="rule"></span>
			<span class="slot" style:width="{mini.w * mini.s}px" style:height="{mini.h * mini.s}px">
				{#each ['thumb', 'back'] as cls (cls)}
					<span class={cls}>
						<span
							class="mini"
							style:width="{mini.w}px"
							style:height="{mini.h}px"
							style:transform="scale({mini.s})">{@render machineWin(false)}</span
						>
					</span>
				{/each}
				<span class="t-time">21:22</span>
			</span>
			<span class="rule grow"></span>
		</div>
	</div>

	{#if geo}
		<svg class="lines" width={geo.w} height={geo.h} aria-hidden="true">
			{#each geo.beams as d (d)}<path class="beam" {d} pathLength="1" />{/each}
			<path class="reach" d={geo.reach} pathLength="1" />
			<path class="ret" d={geo.back} pathLength="1" />
			<path class="ret" d={geo.backHead} pathLength="1" />
		</svg>
	{/if}

	<div class="fly" aria-hidden="true" bind:this={fly}></div>
</div>

<style>
	.hero-pic {
		--accent: var(--color-blue-600);
		--ink: var(--color-zinc-800);
		--dim: var(--color-zinc-500);
		--faint: var(--color-zinc-400);
		--stop: var(--color-red-600);
		--claude: #d97757;
		--rogue: #c0271c;
		position: relative;
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		min-width: 0;
		font-size: 13px;
		color: var(--ink);
	}
	.side,
	.mwin {
		display: flex;
		min-width: 0;
	}

	/* Windows, as in "Your working state" */
	.win {
		position: relative;
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		border: 1px solid var(--rule-strong);
		border-radius: 3px;
		background: var(--surface);
		color: var(--ink);
		font-size: 13px;
	}
	/* The machine's wall. */
	.mwin > .win {
		border-color: var(--ink);
	}
	.title {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 36px;
		padding: 0 14px;
		border-bottom: 1px solid var(--rule);
		border-radius: 2px 2px 0 0;
		background: var(--sunken);
	}
	.mark {
		flex: none;
	}
	.who {
		font-weight: 600;
		white-space: nowrap;
	}
	.dot {
		width: 7px;
		height: 7px;
		margin-left: auto;
		border-radius: 50%;
		background: var(--color-emerald-500);
	}
	.rows {
		margin: 0;
		padding: 10px 0 4px;
		list-style: none;
	}
	.fr {
		position: relative;
		display: flex;
		align-items: center;
		gap: 7px;
		height: 28px;
		padding: 0 14px 0 26px;
		white-space: nowrap;
	}
	.fr.head {
		padding-left: 12px;
		gap: 6px;
	}
	.chev {
		flex: none;
		color: var(--dim);
	}
	.fi {
		position: relative;
		flex: none;
		color: var(--dim);
	}
	.fname {
		position: relative;
		flex: none;
		font-family: var(--font-mono);
		font-size: 12.5px;
	}
	.head .fname {
		font-weight: 600;
	}
	.head .fi {
		color: var(--accent);
	}

	/* The laptop's private things: targeted, never reached. */
	.tg {
		position: absolute;
		inset: 0;
		background: color-mix(in oklab, var(--stop) 10%, var(--surface));
		box-shadow: inset 2px 0 0 var(--stop);
	}
	.laptop .rows {
		padding-bottom: 10px;
	}
	/* Your repo, then the rest of your life. */
	.repo {
		margin-bottom: 7px;
	}
	.repo::after {
		content: '';
		position: absolute;
		left: 12px;
		right: 14px;
		bottom: -4px;
		border-top: 1px solid var(--rule);
	}

	/* Deleted on the machine */
	.xbg {
		position: absolute;
		inset: 0;
		background: color-mix(in oklab, var(--stop) 9%, var(--surface));
	}
	.hero-pic:not(.live) .x .fi,
	.hero-pic:not(.live) .x .fname {
		opacity: 0.45;
	}
	.strike {
		position: absolute;
		left: -2px;
		right: -2px;
		top: calc(50% - 1px);
		border-top: 1.5px solid var(--stop);
		transform-origin: left center;
	}
	.hero-pic:not(.live) .x .fname,
	.hero-pic:not(.live) .x .fi,
	.hero-pic.wrecked .x .fname,
	.hero-pic.wrecked .x .fi {
		color: var(--stop);
	}

	/* The lane: where the agent sits and the skill runs at the wall. */
	.lane {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: flex-end;
		height: 44px;
		margin-top: auto;
		padding: 0 14px 6px;
	}
	.agent {
		position: relative;
		display: grid;
		flex: none;
		color: var(--claude);
	}
	.agent svg {
		grid-area: 1 / 1;
		fill: currentColor;
	}
	.agent .red {
		color: var(--rogue);
	}
	.halo {
		grid-area: 1 / 1;
		margin: -5px;
		border: 1px solid var(--rogue);
		border-radius: 3px;
		background: color-mix(in oklab, var(--rogue) 10%, var(--surface));
	}
	.skill {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 2px 7px;
		border: 1px solid var(--stop);
		border-radius: 3px;
		background: var(--surface);
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--stop);
		white-space: nowrap;
	}
	.skill-in {
		position: absolute;
		left: 8px;
		top: calc(50% - 3px);
		translate: 0 -50%;
		z-index: 3;
	}
	.wallhit {
		position: absolute;
		left: -1.5px;
		top: 2px;
		bottom: 8px;
		border-left: 2px solid var(--stop);
	}
	.stopper {
		position: absolute;
		left: -9px;
		top: calc(50% - 3px);
		translate: 0 -50%;
		z-index: 4;
		display: grid;
		color: var(--stop);
	}
	.shutter {
		position: absolute;
		inset: 0;
		opacity: 0;
		background: color-mix(in oklab, var(--accent) 14%, transparent);
		box-shadow: inset 0 0 0 2px var(--accent);
		pointer-events: none;
	}

	/* The internet */
	.net {
		display: flex;
		align-items: center;
		gap: 10px;
		color: var(--dim);
	}
	.globe {
		flex: none;
	}
	.hero-pic:not(.live) .net .skill {
		opacity: 0.3;
	}

	/* The snapshot rail */
	.rail {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		padding-top: 18px;
		color: var(--dim);
	}
	.cam {
		flex: none;
		margin-top: 2px;
	}
	.rule {
		flex: none;
		width: 14px;
		margin-top: 10px;
		border-top: 1px solid var(--rule-strong);
	}
	.rule.grow {
		display: none;
	}
	.slot {
		position: relative;
		flex: none;
	}
	.thumb,
	.back {
		position: absolute;
		inset: 0;
		overflow: hidden;
		border-radius: 2px;
		outline: 1.5px solid var(--accent);
		background: var(--surface);
		transform-origin: 0 0;
	}
	.back {
		opacity: 0;
		z-index: 5;
	}
	.mini {
		position: absolute;
		left: 0;
		top: 0;
		display: flex;
		transform-origin: 0 0;
		pointer-events: none;
	}
	.t-time {
		position: absolute;
		left: 0;
		top: calc(100% + 4px);
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--dim);
	}

	/* The command between the panels */
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
		height: 16px;
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

	/* Connectors */
	.lines {
		position: absolute;
		left: 0;
		top: 0;
		z-index: 2;
		overflow: visible;
		pointer-events: none;
	}
	.lines path {
		fill: none;
		stroke-width: 1.5;
		stroke-dasharray: 1;
		stroke-dashoffset: 0;
	}
	.lines .beam,
	.lines .reach {
		stroke: var(--rogue);
	}
	.lines .ret {
		stroke: var(--accent);
	}

	/* The repo row in flight (animated only). */
	.fly {
		position: absolute;
		inset: 0;
		z-index: 3;
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

	/* The miniature is the machine as the snapshot took it: whole. */
	.hero-pic .slot .mini .x .fi,
	.hero-pic .slot .mini .x .fname {
		opacity: 1;
		color: var(--ink);
	}
	.hero-pic .slot .mini .x .fi {
		color: var(--dim);
	}
	.mini .xbg,
	.mini .strike,
	.mini .agent .red {
		display: none;
	}

	/* Phone: stacked; the internet sits beside the rail. */
	.hero-pic {
		grid-template-columns: minmax(0, 1fr) auto;
		grid-template-areas: 'lap lap' 'hop hop' 'win win' 'rail net';
		column-gap: 16px;
	}
	.machine {
		display: contents;
	}
	.laptop {
		grid-area: lap;
	}
	.hop {
		grid-area: hop;
	}
	.mwin {
		grid-area: win;
	}
	.rail {
		grid-area: rail;
		min-width: 0;
		padding-bottom: 20px;
	}
	.net {
		grid-area: net;
		padding-top: 28px;
		align-self: start;
	}

	@media (min-width: 768px) {
		.hero-pic {
			grid-template-columns: minmax(0, 1fr) auto minmax(0, 1.25fr) auto;
			grid-template-areas: 'lap hop win net' '. . rail .';
			column-gap: 0;
		}
		.hop {
			flex-direction: row;
			align-self: start;
			margin-top: 49px;
			padding: 0 8px;
		}
		.wire {
			width: 16px;
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
		.rail {
			padding-bottom: 0;
		}
		.t-time {
			left: calc(100% + 8px);
			top: 2px;
		}
		.rule.grow {
			display: block;
			flex: 1;
			min-width: 10px;
			margin-left: 50px;
		}
		.net {
			flex-direction: column;
			align-self: end;
			gap: 8px;
			padding: 0 0 13px 44px;
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
		.title,
		.skill,
		.t-time {
			font-size: 11px;
		}
		.chip {
			font-size: 12px;
		}
	}
	@media (prefers-color-scheme: dark) {
		.hero-pic {
			--accent: var(--color-blue-400);
			--ink: var(--color-zinc-200);
			--dim: var(--color-zinc-400);
			--faint: var(--color-zinc-600);
			--stop: var(--color-red-400);
			--rogue: #ff4b3e;
		}
		.mwin > .win {
			border-color: var(--color-zinc-400);
		}
	}
</style>
