<!--
  The agent's browser, and you in it. A real run on the `recruiting` repose
  machine (the owner's job-alerts app), 2026-09-26.

  Top: the machine's Chromium as the desktop shows it, captured through
  `repose open --desktop` (noVNC canvas, 1x) while the run below happened: the
  toolbar and the page, cropped out of the window (static/landing/browser-*).
  The window was 586px wide at 125% zoom so the text survives the scale-down.
  States, in the order they happened: about:blank; /feedback loaded; the bug
  report typed into the text box; Bug selected; then, from the desktop view
  on this box (the laptop side), a click in the text box and " Only with the
  Backend filter on." typed by hand (three captures while typing). The agent
  read that sentence back with its next page snapshot.

  Bottom: rows of that agent session (Claude Code with the playwright MCP
  server, on the machine in tmux), verbatim `tmux capture-pane -p -e -J` of
  its transcript view (ctrl+o): the prompt, and for each browser call the
  call row and the Playwright code or result row under it, cropped at the
  frame's right edge; the rows between them (### headings, ``` fences, page
  snapshots, a click that timed out on the hidden radio) are cropped out.
  OSC 8 hyperlink wrappers are dropped; they carry no visible text.

  The pan of the page, the blue focus box, the command chip and the pointer
  are the illustration; everything inside the page and terminal is captured.
  anime.js plays it when the card is on screen; without JS, or under
  prefers-reduced-motion, the last beat is shown still.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	const ROWS = [
		'\u001b[38;5;239m\u001b[48;5;237m❯ \u001b[38;5;231mTest the feedback form at http://localhost:5173/feedback with the playwright browser tools, in the browser window that is already open. Choose Bug, type a one-line bug report into the text box, and\u001b[39m',
		'\u001b[38;5;114m●\u001b[39m \u001b[1mplaywright - Navigate to a URL (MCP)\u001b[0m(url: "http://localhost:5173/feedback")',
		"     await page.goto('\u001b[94mhttp://localhost:5173/feedback\u001b[39m');",
		'\u001b[38;5;114m●\u001b[39m \u001b[1mplaywright - Type text (MCP)\u001b[0m(target: "f1e32", element: "Your feedback textbox", text: "The Roles page filter resets when I navigate back from a role.")',
		"     await page.getByRole('textbox', { name: 'Your feedback' }).fill('The Roles page filter resets when I navigate back from a role.');",
		'\u001b[38;5;114m●\u001b[39m \u001b[1mplaywright - Get console messages (MCP)\u001b[0m(level: "warning", all: true)',
		'     Total messages: 2 (Errors: 0, Warnings: 0)',
		'\u001b[38;5;114m●\u001b[39m \u001b[1mplaywright - Click (MCP)\u001b[0m(target: "f1e18", element: "Bug label")',
		"     await page.getByText('Bug').click();"
	];

	// The xterm-256 and bright colours this capture uses.
	const X256: Record<number, string> = {
		114: '#87d787',
		231: '#ffffff',
		237: '#3a3a3a',
		239: '#8a8a8a',
		246: '#949494'
	};
	const BRIGHT_BLUE = '#8fb3ff';

	// An SGR escape: ESC [ params m.
	const SGR = new RegExp(`${String.fromCharCode(27)}\\[([0-9;]*)m`, 'g');

	type Seg = { text: string; style: string };
	type Row = { segs: Seg[]; bg: string };
	function parse(line: string): Row {
		const segs: Seg[] = [];
		let fg = '';
		let rowBg = '';
		let bold = false;
		let pos = 0;
		const push = (text: string) => {
			if (!text) return;
			let style = fg ? `color:${fg};` : '';
			if (bold) style += 'font-weight:700;color:' + (fg || '#f2f2ef') + ';';
			segs.push({ text, style });
		};
		for (const m of line.matchAll(SGR)) {
			push(line.slice(pos, m.index));
			pos = m.index + m[0].length;
			const ps = (m[1] || '0').split(';').map(Number);
			for (let i = 0; i < ps.length; i++) {
				const p = ps[i];
				if (p === 0) [fg, bold] = ['', false];
				else if (p === 1) bold = true;
				else if (p === 22) bold = false;
				else if (p === 39) fg = '';
				else if (p === 94) fg = BRIGHT_BLUE;
				else if (p === 38 && ps[i + 1] === 5) {
					fg = X256[ps[i + 2]] ?? '';
					i += 2;
				} else if (p === 48 && ps[i + 1] === 5) {
					rowBg = X256[ps[i + 2]] ?? '';
					i += 2;
				}
			}
		}
		push(line.slice(pos));
		return { segs, bg: rowBg };
	}
	const rows = ROWS.map(parse);

	// Geometry, in capture pixels of the 574px-wide crops. The toolbar is
	// 48px tall; the page crops start right under it.
	const TB = 48;
	const URLBAR = { x: 127, y: 6, w: 362, h: 35 };
	const TEXTBOX = { x: 29, y: 204, w: 511, h: 209 };
	const BUG = { x: 28, y: 1, w: 67, h: 37 };
	// Where the pointer is drawn: in the text box, under the text (the real
	// click, at y 350, sits below the frame's crop).
	const POINTER = { x: 330, y: 285 };
	const PAN = { nav: 150, typed: 150, bug: 0, you: 150 };

	const label =
		"The agent's browser on a cloud machine, next to the agent's log. The agent opens the app's feedback page, " +
		'types a bug report into the text box, reads the console (0 errors, 0 warnings) and clicks Bug; each Playwright ' +
		'call in the log changes the page above it. Then repose open --desktop shows the same browser on your laptop, ' +
		'and your pointer clicks into the text box and adds a sentence.';

	let pic: HTMLDivElement;

	onMount(() => {
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

		let dead = false;
		let visible = false;
		let tl: { pause(): unknown; play(): unknown } | null = null;
		let io: IntersectionObserver | undefined;

		const q = (s: string) => Array.from(pic.querySelectorAll<HTMLElement>(s));
		const one = (s: string) => pic.querySelector<HTMLElement>(s)!;

		import('animejs').then(({ createTimeline, utils }) => {
			if (dead) return;

			const rowEls = q('.tr');
			const list = one('.rows');
			const rowH = () => rowEls[0].getBoundingClientRect().height;
			// Keep row `last` at the bottom of the terminal.
			const offset = (last: number) => (rowEls.length - 1 - last) * rowH();

			function pre() {
				utils.set(q('.layer'), { opacity: 0 });
				utils.set(q('.l-blank, .tb-blank'), { opacity: 1 });
				utils.set(pic, { '--pan': PAN.nav });
				utils.set(one('.ring'), {
					opacity: 0,
					'--rx': URLBAR.x,
					'--ry': URLBAR.y,
					'--rw': URLBAR.w,
					'--rh': URLBAR.h
				});
				utils.set(one('.chip'), { opacity: 0, scale: 0.85 });
				one('.chip').classList.remove('fire');
				utils.set(one('.pointer'), { opacity: 0 });
				utils.set(rowEls, { opacity: 0 });
				utils.set(rowEls[0], { opacity: 1 });
				utils.set(list, { y: offset(0) });
				rowEls.forEach((r) => r.classList.remove('new'));
			}

			function cycle() {
				if (dead) return;
				if (!visible) {
					tl = null;
					return;
				}
				pre();

				const t = createTimeline({ autoplay: false, onComplete: () => cycle() });

				// A browser call lands in the log: its two rows rise in, the
				// older rows dim, the call row is marked while it acts.
				const log = (first: number, at: number) => {
					t.call(() => {
						rowEls.forEach((r) => r.classList.remove('new'));
						rowEls[first].classList.add('new');
					}, at)
						.add(list, { y: offset(first + 1), duration: 520, ease: 'outCubic' }, at)
						.add(rowEls.slice(0, first), { opacity: 0.45, duration: 400 }, at)
						.add([rowEls[first], rowEls[first + 1]], { opacity: [0, 1], duration: 380 }, at + 60);
				};
				const box = (r: { x: number; y: number; w: number; h: number }, pan: number, at: number) =>
					t.add(
						one('.ring'),
						{
							'--rx': r.x,
							'--ry': TB + r.y - pan,
							'--rw': r.w,
							'--rh': r.h,
							duration: 700,
							ease: 'inOutCubic'
						},
						at
					);
				const show = (sel: string, at: number, duration = 300) =>
					t.add(q(sel), { opacity: [0, 1], duration, ease: 'linear' }, at);
				const pan = (to: number, at: number, duration = 800) =>
					t.add(pic, { '--pan': to, duration, ease: 'inOutCubic' }, at);

				// 1. Navigate.
				log(1, 700);
				t.add(one('.ring'), { opacity: [0, 1], duration: 250 }, 800);
				show('.tb-live', 1150, 200);
				show('.l-nav', 1150, 350);

				// 2. Type into the text box (the page scrolled it into view).
				log(3, 2900);
				box(TEXTBOX, PAN.typed, 3000);
				pan(PAN.typed, 3100, 10);
				show('.l-typed', 3100, 300);

				// 3. Read the console: nothing on the page moves.
				log(5, 4900);
				t.add(one('.ring'), { opacity: 0, duration: 300 }, 5000);

				// 4. Click Bug.
				log(7, 6600);
				pan(PAN.bug, 6700, 800);
				t.set(
					one('.ring'),
					{ '--rx': BUG.x, '--ry': TB + BUG.y - PAN.bug, '--rw': BUG.w, '--rh': BUG.h },
					7500
				).add(one('.ring'), { opacity: [0, 1], duration: 250 }, 7520);
				show('.l-bug', 7800, 220);

				// 5. You, from the laptop: the desktop view opens on the same
				// window and your pointer types into it.
				t.add(one('.ring'), { opacity: 0, duration: 300 }, 8800);
				rowEls.forEach((r) => t.call(() => r.classList.remove('new'), 8800));
				t.add(rowEls, { opacity: 0.45, duration: 500 }, 8800);
				t.add(
					one('.chip'),
					{ opacity: [0, 1], scale: [0.85, 1], duration: 320, ease: 'outBack' },
					9000
				);
				t.call(() => one('.chip').classList.add('fire'), 9450);
				t.call(() => one('.chip').classList.remove('fire'), 9950);
				pan(PAN.you, 9300, 900);

				const k = pic.querySelector('.br')!.getBoundingClientRect().width / 574;
				const c = one('.chip').getBoundingClientRect();
				const b = pic.querySelector('.br')!.getBoundingClientRect();
				const sx = (c.left - b.left + c.width * 0.3) / k;
				const sy = (c.bottom - b.top - 4) / k;
				const ex = POINTER.x;
				const ey = TB + POINTER.y - PAN.you;
				t.set(one('.pointer'), { '--px': sx, '--py': sy }, 0)
					.add(one('.pointer'), { opacity: [0, 1], duration: 200 }, 10000)
					.add(
						one('.pointer'),
						{ '--px': ex, '--py': ey, duration: 900, ease: 'inOutCubic' },
						10050
					)
					.add(one('.pointer'), { scale: [1, 0.86, 1], duration: 220 }, 11000);
				show('.l-t0', 11080, 120);
				show('.l-t1', 11700, 160);
				show('.l-t2', 12400, 160);

				// Rest on it, then clear and go again.
				t.add(q('.layer, .chip, .pointer'), { opacity: 0, duration: 450, ease: 'inQuad' }, 17200)
					.add(rowEls, { opacity: 0, duration: 450 }, 17200)
					.add({ duration: 300 }, 17700);

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
			io.observe(pic);
		});

		return () => {
			dead = true;
			io?.disconnect();
			tl?.pause();
		};
	});
</script>

<div class="min-w-0">
	<div class="frame h-60 overflow-hidden rounded-xs border border-[var(--rule)] bg-[var(--sunken)]">
		<div class="stage" role="img" aria-label={label} bind:this={pic}>
			<div class="br" aria-hidden="true">
				<div class="tb">
					<img
						class="layer tb-blank"
						src="/landing/browser-tb-blank.webp"
						width="574"
						height="48"
						alt=""
					/>
					<img
						class="layer tb-live on"
						src="/landing/browser-tb.webp"
						width="574"
						height="48"
						alt=""
					/>
				</div>
				<div class="vp">
					<div class="pg">
						<div class="layer l-blank"></div>
						<img
							class="layer l-nav"
							src="/landing/browser-p-nav.webp"
							width="574"
							height="536"
							alt=""
						/>
						<img
							class="layer l-typed"
							src="/landing/browser-p-typed.webp"
							width="574"
							height="536"
							alt=""
						/>
						<img
							class="layer l-bug"
							src="/landing/browser-p-bug.webp"
							width="574"
							height="536"
							alt=""
						/>
						<img
							class="layer l-t0"
							src="/landing/browser-p-t0.webp"
							width="574"
							height="536"
							alt=""
						/>
						<img
							class="layer l-t1"
							src="/landing/browser-p-t1.webp"
							width="574"
							height="536"
							alt=""
						/>
						<img
							class="layer l-t2 on"
							src="/landing/browser-p-t2.webp"
							width="574"
							height="536"
							alt=""
						/>
					</div>
				</div>
				<i class="ring"></i>
				<span class="chip">repose open --desktop</span>
				<svg class="pointer" viewBox="0 0 12 18" aria-hidden="true">
					<path
						d="M1 1v13.2l3.3-3.1 2.2 5.1 2.1-.9-2.2-5h4.6z"
						fill="#111"
						stroke="#fff"
						stroke-width="1.1"
						stroke-linejoin="round"
					/>
				</svg>
			</div>
			<div class="term" aria-hidden="true">
				<div class="clip">
					<div class="rows">
						{#each rows as row, i (i)}
							<div class="tr" style={row.bg ? `background:${row.bg}` : ''}>
								{#each row.segs as seg, j (j)}<span style={seg.style}>{seg.text}</span>{/each}
							</div>
						{/each}
					</div>
				</div>
			</div>
		</div>
	</div>
	<h3 class="mt-5 text-lg font-semibold">Watch the agent use the browser</h3>
	<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
		The agent drives Chromium on the machine and reads the console. <code
			class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
			>repose open --desktop</code
		> shows you the same window, and you can take over.
	</p>
</div>

<style>
	.frame {
		container-type: inline-size;
	}
	.stage {
		/* the chip and ring sit on the (light) page, in either scheme */
		--accent: var(--color-blue-600);
		/* one capture pixel, at this frame's width */
		--k: calc(100cqw / 574);
		--pan: 150;
		position: relative;
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	/* The browser: flush with the frame's top and sides, a crop of the window. */
	.br {
		position: relative;
		flex: 1;
		min-height: 0;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		background: #fff;
	}
	.tb {
		position: relative;
		flex: none;
		height: calc(var(--k) * 48);
		border-bottom: 1px solid #d9dce0;
		background: #fff;
	}
	.vp {
		position: relative;
		flex: 1;
		overflow: hidden;
	}
	.pg {
		position: absolute;
		left: 0;
		right: 0;
		top: calc(var(--k) * var(--pan) * -1);
		height: calc(var(--k) * 536);
	}
	.layer {
		position: absolute;
		inset: 0;
		width: 100%;
		max-width: none;
		height: 100%;
		opacity: 0;
	}
	.layer.on {
		opacity: 1;
	}
	.l-blank {
		background: #fff;
	}

	/* The agent's focus, travelling to what it acts on. */
	.ring {
		--rx: 29;
		--ry: 0;
		--rw: 511;
		--rh: 209;
		position: absolute;
		left: calc(var(--k) * var(--rx) - 3px);
		top: calc(var(--k) * var(--ry) - 3px);
		width: calc(var(--k) * var(--rw) + 6px);
		height: calc(var(--k) * var(--rh) + 6px);
		border: 2px solid var(--color-blue-500);
		border-radius: 3px;
		opacity: 0;
		pointer-events: none;
	}

	/* You: the command, and your pointer in the same window. */
	.chip {
		position: absolute;
		right: 10px;
		top: calc(var(--k) * 48 + 10px);
		padding: 2px 8px;
		border: 1px solid var(--accent);
		border-radius: 3px;
		background: #fff;
		font-family: var(--font-mono);
		font-size: 12px;
		font-weight: 600;
		line-height: 18px;
		color: var(--accent);
		white-space: nowrap;
		transition:
			background-color 0.18s,
			color 0.18s;
	}
	.chip:global(.fire) {
		background: var(--accent);
		color: #fff;
	}
	.pointer {
		--px: 330;
		--py: 183;
		position: absolute;
		left: calc(var(--k) * var(--px) - 1px);
		top: calc(var(--k) * var(--py) - 1px);
		width: 13px;
		height: 19px;
		transform-origin: 1px 1px;
	}

	/* The agent's log. */
	.term {
		position: relative;
		flex: none;
		height: 52px;
		overflow: hidden;
		border-top: 1px solid #2a2a28;
		background: #0d0d0c;
		color: #d4d4d0;
		font-family: 'JetBrains Mono', 'SF Mono', Menlo, 'DejaVu Sans Mono', Consolas, monospace;
		font-size: 11px;
		line-height: 14px;
		font-variant-ligatures: none;
	}
	.clip {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 5px;
		height: 42px;
		overflow: hidden;
	}
	.rows {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
	}
	.tr {
		padding: 0 12px;
		white-space: pre;
		overflow: hidden;
		transition: box-shadow 0.3s;
	}
	.tr:not(:nth-last-child(-n + 3)) {
		opacity: 0.45;
	}
	.tr:global(.new) {
		box-shadow: inset 2px 0 0 var(--color-blue-400);
	}

	@container (max-width: 400px) {
		.term {
			font-size: 10px;
			line-height: 13px;
			height: 49px;
		}
		.clip {
			height: 39px;
		}
		.tr {
			padding: 0 10px;
		}
		.chip {
			font-size: 11px;
			line-height: 16px;
		}
	}
</style>
