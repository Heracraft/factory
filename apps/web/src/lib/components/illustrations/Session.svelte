<!--
  The hero: a recording, not a drawing. A real tmux session was captured
  every 200 ms on a copy of a real project (ops/dev/record-hero.md says how):
  Claude Code on the left fixing a real bug, the app's Vite dev server on the
  right logging the reloads as Claude saves files. session.json holds the
  frames (ANSI converted to styled runs, lines de-duplicated); this component
  only plays them back, then shows what the laptop's terminal prints after
  Ctrl-b d. The ending after that is drawn here, not recorded: the
  notification a phone gets, `repose attach` typed on the laptop, and the
  same session back on Claude's finished answer. Playback starts where the
  prompt is typed, since the recording's first ten seconds are an idle
  screen. Below 640px only Claude's pane is shown, so its text stays
  readable. Reduced motion shows Claude's finished answer.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import data from './session.json';

	type Seg = [string, number];
	type Frame = { at: number; l?: number[]; r?: number[]; b?: number[]; full?: number[] };
	type Style = {
		fg?: string;
		bg?: string;
		b?: number;
		d?: number;
		i?: number;
		u?: number;
		r?: number;
	};

	const styles = data.styles as Style[];
	const lines = data.lines as Seg[][];
	const seq = data.seq as Frame[];
	const LEFT_COLS = 87;
	const RIGHT_COLS = 62;
	const ROWS = 36;
	const FG = '#d4d4d0';
	const BG = '#0d0d0c';

	function css(ix: number): string {
		const s = styles[ix] ?? {};
		let fg = s.fg ?? FG;
		let bg = s.bg;
		if (s.r) [fg, bg] = [bg ?? BG, fg];
		let out = `color:${fg};`;
		if (bg) out += `background:${bg};`;
		if (s.b) out += 'font-weight:600;';
		if (s.d) out += 'opacity:.62;';
		if (s.i) out += 'font-style:italic;';
		if (s.u) out += 'text-decoration:underline;';
		return out;
	}

	// The last frame of the session proper: Claude's finished answer.
	const lastSession = seq.findLastIndex((f) => f.l);

	// The timeline, in the recording's milliseconds. The recording opens on
	// ten idle seconds, so the loop starts just before the prompt is typed.
	// Everything from NOTIFY on is drawn here rather than recorded.
	const START = 9800;
	const DETACH = seq[seq.length - 1].at;
	const NOTIFY = DETACH + 1500;
	const TYPE_AT = DETACH + 3800;
	const TYPED = 'repose attach';
	const KEY_MS = 75;
	const ATTACH = TYPE_AT + TYPED.length * KEY_MS + 450;
	const END = ATTACH + 5500;

	const chapters = [
		{ label: 'Claude works', at: START },
		{ label: 'You detach', at: DETACH },
		{ label: 'You come back', at: NOTIFY }
	];

	// Below this width only Claude's pane is drawn (87 columns plus padding);
	// above it, the whole 150-column tmux window.
	const COMPACT_BELOW = 640;
	const FULL_W = 1098;
	const LEFT_W = 644;

	let t = $state(START);
	let width = $state(FULL_W);
	let compact = $derived(width < COMPACT_BELOW);
	let stageW = $derived(compact ? LEFT_W : FULL_W);
	let scale = $derived(width / stageW);
	let wrap: HTMLDivElement;
	let jump: (ms: number) => void = (ms) => (t = ms);

	let index = $derived.by(() => {
		if (t >= ATTACH) return lastSession;
		let lo = 0;
		let hi = seq.length - 1;
		while (lo < hi) {
			const mid = (lo + hi + 1) >> 1;
			if (seq[mid].at <= t) lo = mid;
			else hi = mid - 1;
		}
		return lo;
	});
	let frame = $derived(seq[index]);
	let notified = $derived(t >= NOTIFY);
	let typed = $derived(
		t >= TYPE_AT && t < ATTACH
			? TYPED.slice(0, Math.min(TYPED.length, Math.floor((t - TYPE_AT) / KEY_MS) + 1))
			: ''
	);
	let chapter = $derived(chapters.findLastIndex((c) => c.at <= t));

	onMount(() => {
		// Deferred a frame: switching to the compact layout changes the page's
		// height, and so the scrollbar and this element's width, inside the
		// same observer callback otherwise.
		const ro = new ResizeObserver(([e]) => {
			const w = e.contentRect.width;
			requestAnimationFrame(() => (width = w));
		});
		ro.observe(wrap);
		let seeking = false;
		(window as unknown as { __heroSeek?: (ms: number) => void }).__heroSeek = (ms) => {
			seeking = true;
			t = ms;
		};
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
			t = ATTACH;
			return () => ro.disconnect();
		}
		// Start playing only when the terminal is on screen, from the top.
		let raf = 0;
		let t0 = 0;
		const tick = (now: number) => {
			if (!t0) t0 = now;
			if (!seeking) t = START + ((now - t0) % (END - START));
			raf = requestAnimationFrame(tick);
		};
		jump = (ms) => {
			seeking = false;
			t = ms;
			t0 = performance.now() - (ms - START);
		};
		const io = new IntersectionObserver(([e]) => {
			if (e.isIntersecting && !raf) raf = requestAnimationFrame(tick);
			else if (!e.isIntersecting && raf) {
				cancelAnimationFrame(raf);
				raf = 0;
				t0 = 0;
			}
		});
		io.observe(wrap);
		return () => {
			cancelAnimationFrame(raf);
			io.disconnect();
			ro.disconnect();
		};
	});

	// Block elements (the Claude Code mascot is drawn with them) are painted
	// as quarter-cell fills, the way terminal emulators draw them, instead of
	// trusting a web font to carry the glyphs: Google Fonts' subsets drop them.
	const QUADS: Record<string, string> = {
		'▘': '1000',
		'▝': '0100',
		'▖': '0010',
		'▗': '0001',
		'▀': '1100',
		'▄': '0011',
		'▌': '1010',
		'▐': '0101',
		'▚': '1001',
		'▞': '0110',
		'▙': '1011',
		'▛': '1110',
		'▜': '1101',
		'▟': '0111',
		'█': '1111'
	};
	type Piece = { text: string; quad?: string };
	function pieces(text: string): Piece[] {
		if (!/[\u2580-\u259f]/.test(text)) return [{ text }];
		const out: Piece[] = [];
		for (const ch of text) {
			if (QUADS[ch]) out.push({ text: ch, quad: QUADS[ch] });
			else if (out.length && !out[out.length - 1].quad) out[out.length - 1].text += ch;
			else out.push({ text: ch });
		}
		return out;
	}
	function quadFill(style: number): { fg: string; bg: string } {
		const st = styles[style] ?? {};
		// Pure black behind a block is the terminal's own background; painting
		// it would draw the half-pixel overlap between cells as a dark seam.
		const bg = !st.bg || st.bg === '#000000' ? 'transparent' : st.bg;
		return { fg: st.fg ?? FG, bg };
	}

	function rows(ids: number[] | undefined, count = ROWS): number[] {
		const r = (ids ?? []).slice(0, count);
		while (r.length < count) r.push(-1);
		return r;
	}
</script>

{#snippet seg(text: string, s: number)}{#each pieces(text) as p, k (k)}{#if p.quad}{@const q =
				quadFill(s)}<svg
				class="blk"
				viewBox="0 0 2 2"
				preserveAspectRatio="none"
				shape-rendering="crispEdges"
				overflow="visible"
				style="background:{q.bg}"
				>{#each [...p.quad] as bit, n (n)}{#if bit === '1'}<rect
							x={n % 2}
							y={n > 1 ? 1 : 0}
							width="1.12"
							height="1.02"
							fill={q.fg}
						/>{/if}{/each}</svg
			>{:else}<span style={css(s)}>{p.text}</span>{/if}{/each}{/snippet}

{#snippet pane(ids: number[] | undefined, cols: number, count = ROWS)}
	<div class="pane" style="width: {cols}ch">
		{#each rows(ids, count) as id, k (k)}
			<div class="trow">
				{#if id >= 0}
					{#each lines[id] as [text, s], j (j)}{@render seg(text, s)}{/each}
				{/if}
			</div>
		{/each}
	</div>
{/snippet}

<div>
	<div
		bind:this={wrap}
		class="session relative w-full"
		style="aspect-ratio: {stageW} / 630"
		role="img"
		aria-label="A tmux session on a repose machine: Claude Code on the left fixes a hardcoded graduation year in a SvelteKit app, reading the component, updating the client and server validation and running the tests; the app's Vite dev server on the right reloads as each file is saved. Then the laptop detaches, a notification says Claude finished, and repose attach brings back the same session."
	>
		<div class="stage" style="width: {stageW}px; transform: scale({scale})" aria-hidden="true">
			<div class="titlebar">
				<span class="dots"><i></i><i></i><i></i></span>
				<span class="title">{frame.full ? '~/code/recruiting' : 'ssh izma.repose'}</span>
			</div>
			<div class="screen">
				{#if frame.full}
					<div class="pane" style="width: 150ch">
						{#each rows(frame.full) as id, k (k)}
							<div class="trow">
								{#if id >= 0}
									{#each lines[id] as [text, s], j (j)}{@render seg(text, s)}{/each}
								{/if}
								{#if k === frame.full.length - 1}<span class="typed">{typed}</span><span
										class="cursor"
									></span>{/if}
							</div>
						{/each}
					</div>
				{:else}
					<div class="panes">
						{@render pane(frame.l, LEFT_COLS)}
						{#if !compact}
							<div class="divider"></div>
							{#if frame.b}
								<!-- the right pane split in two, as tmux draws it: 23 rows,
								     a border row, 12 rows -->
								<div class="stack">
									{@render pane(frame.r, RIGHT_COLS, 23)}
									<div class="hdivider"></div>
									{@render pane(frame.b, RIGHT_COLS, 12)}
								</div>
							{:else}
								{@render pane(frame.r, RIGHT_COLS)}
							{/if}
						{/if}
					</div>
					<div class="status">
						<span>[izma] 0:dev*</span>
						<span>{compact ? '18:41' : '"repose-guest" 18:41 23-Sep-26'}</span>
					</div>
				{/if}
			</div>
			<!-- What a phone shows when Claude's Stop hook fires (ntfy). The
			     summary is two sentences from Claude's own answer above. -->
			<div class="notice" class:shown={notified && t < ATTACH}>
				<div class="notice-head"><span>repose · izma</span><span>now</span></div>
				<div class="notice-title">claude finished</div>
				<div class="notice-body">
					All 13 tests pass, and svelte-check reports 0 errors. Nothing is committed.
				</div>
			</div>
		</div>
	</div>
	<div class="chapters" role="group" aria-label="Jump to a part of the recording">
		{#each chapters as c, i (c.label)}
			<button
				type="button"
				class="chapter"
				class:current={i === chapter}
				aria-pressed={i === chapter}
				onclick={() => jump(c.at)}
			>
				<span
					class="bar"
					style="--p: {i < chapter
						? 1
						: i === chapter
							? Math.min(1, (t - c.at) / ((chapters[i + 1]?.at ?? END) - c.at))
							: 0}"
				></span>
				<span class="label"><span class="num">{i + 1}</span>{c.label}</span>
			</button>
		{/each}
	</div>
</div>

<style>
	.session {
		overflow: hidden;
		border: 1px solid #2a2a28;
		background: #0d0d0c;
	}
	.stage {
		position: absolute;
		left: 0;
		top: 0;
		height: 630px;
		transform-origin: 0 0;
		font-family: 'JetBrains Mono', 'SF Mono', Menlo, 'DejaVu Sans Mono', Consolas, monospace;
		font-size: 12px;
		font-variant-ligatures: none;
		color: #d4d4d0;
		background: #0d0d0c;
	}
	.titlebar {
		position: relative;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #1c1c1b;
		border-bottom: 1px solid #2a2a28;
		font-family: ui-sans-serif, system-ui, sans-serif;
		font-size: 12px;
		color: #9a9a95;
	}
	.dots {
		position: absolute;
		left: 12px;
		display: flex;
		gap: 7px;
	}
	.dots i {
		width: 11px;
		height: 11px;
		border-radius: 50%;
		background: #3a3a38;
	}
	.screen {
		padding: 6px 8px 0;
	}
	.panes {
		display: flex;
		height: calc(36 * 16px);
	}
	.pane {
		flex: none;
		overflow: hidden;
	}
	.trow {
		height: 16px;
		line-height: 16px;
		white-space: pre;
		overflow: hidden;
	}
	/* Half a pixel of overlap each way, so fractional cell widths under the
	   stage's scale do not leave hairline seams between blocks. */
	.blk {
		display: inline-block;
		width: calc(1ch + 0.6px);
		height: 16.6px;
		margin: -0.3px -0.6px -0.3px 0;
		vertical-align: top;
	}
	.stack {
		display: flex;
		flex-direction: column;
	}
	.hdivider {
		height: 16px;
		background: linear-gradient(#3f8a5e, #3f8a5e) 0 50% / 100% 1px no-repeat;
	}
	.divider {
		flex: none;
		width: 1ch;
		background: linear-gradient(#3f8a5e, #3f8a5e) center / 1px 100% no-repeat;
	}
	.status {
		display: flex;
		justify-content: space-between;
		height: 16px;
		line-height: 16px;
		margin: 2px -8px 0;
		padding: 0 8px;
		background: #1f9d55;
		color: #0b0b0b;
		white-space: pre;
	}
	.typed {
		color: #d4d4d0;
	}
	.cursor {
		display: inline-block;
		width: 1ch;
		height: 14px;
		vertical-align: -2px;
		background: #d4d4d0;
		animation: blink 1s steps(1) infinite;
	}
	@keyframes blink {
		50% {
			opacity: 0;
		}
	}
	.notice {
		position: absolute;
		top: 44px;
		right: 20px;
		width: 340px;
		padding: 12px 14px;
		border-radius: 4px;
		background: #f4f4f2;
		color: #1c1c1b;
		font-family: ui-sans-serif, system-ui, sans-serif;
		font-size: 14px;
		line-height: 1.4;
		border: 1px solid #cfcfcb;
		opacity: 0;
		transform: translateY(-16px);
		transition:
			opacity 0.35s ease,
			transform 0.35s ease;
	}
	.notice.shown {
		opacity: 1;
		transform: none;
	}
	.notice-head {
		display: flex;
		justify-content: space-between;
		font-size: 12px;
		color: #6b6b66;
	}
	.notice-title {
		margin-top: 4px;
		font-weight: 600;
	}
	.notice-body {
		color: #3a3a38;
	}
	.chapters {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
		margin-top: 10px;
	}
	.chapter {
		cursor: pointer;
		text-align: left;
		padding: 0;
		background: none;
		border: 0;
		color: var(--color-zinc-600);
		font-size: 13px;
	}
	.chapter:hover,
	.chapter.current {
		color: var(--color-zinc-900);
	}
	.chapter.current {
		font-weight: 500;
	}
	@media (prefers-color-scheme: dark) {
		.chapter {
			color: var(--color-zinc-400);
		}
		.chapter:hover,
		.chapter.current {
			color: var(--color-zinc-100);
		}
	}
	.bar {
		display: block;
		height: 2px;
		margin-bottom: 6px;
		background: linear-gradient(currentColor, currentColor) 0 0 / calc(var(--p) * 100%) 100%
			no-repeat var(--rule-strong);
	}
	.num {
		margin-right: 6px;
		font-family: var(--font-mono);
		font-size: 12px;
		opacity: 0.7;
	}
</style>
