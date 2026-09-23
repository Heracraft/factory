<!--
  The hero: a recording, not a drawing. A real tmux session was captured
  every 200 ms on a copy of a real project (ops/dev/record-hero.md says how):
  Claude Code on the left fixing a real bug, the app's Vite dev server on the
  right logging the reloads as Claude saves files. session.json holds the
  frames (ANSI converted to styled runs, lines de-duplicated); this component
  only plays them back, then shows what the laptop's terminal prints after
  Ctrl-b d. Reduced motion shows Claude's finished answer.
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
	const T = data.T as number;
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

	let t = $state(0);
	let scale = $state(1);
	let wrap: HTMLDivElement;

	let index = $derived.by(() => {
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

	onMount(() => {
		const ro = new ResizeObserver(([e]) => (scale = e.contentRect.width / 1098));
		ro.observe(wrap);
		let seeking = false;
		(window as unknown as { __heroSeek?: (ms: number) => void }).__heroSeek = (ms) => {
			seeking = true;
			t = ms;
		};
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
			t = seq[lastSession].at;
			return () => ro.disconnect();
		}
		// Start playing only when the terminal is on screen, from the top.
		let raf = 0;
		let t0 = 0;
		const tick = (now: number) => {
			if (!t0) t0 = now;
			if (!seeking) t = (now - t0) % T;
			raf = requestAnimationFrame(tick);
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

<div
	bind:this={wrap}
	class="session relative w-full"
	style="aspect-ratio: 1098 / 630"
	role="img"
	aria-label="A tmux session on a repose machine: Claude Code on the left fixes a hardcoded graduation year in a SvelteKit app, reading the component, updating the client and server validation and running the tests; the app's Vite dev server on the right reloads as each file is saved. Then the session is detached and keeps running."
>
	<div class="stage" style="transform: scale({scale})" aria-hidden="true">
		<div class="titlebar">
			<span class="dots"><i></i><i></i><i></i></span>
			<span class="title">ssh izma.repose</span>
		</div>
		<div class="screen">
			{#if frame.full}
				<div class="pane" style="width: 150ch">
					{#each rows(frame.full) as id, k (k)}
						<div class="trow">
							{#if id >= 0}
								{#each lines[id] as [text, s], j (j)}{@render seg(text, s)}{/each}
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<div class="panes">
					{@render pane(frame.l, LEFT_COLS)}
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
				</div>
				<div class="status">
					<span>[izma] 0:dev*</span>
					<span>"repose-guest" 18:41 23-Sep-26</span>
				</div>
			{/if}
		</div>
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
		width: 1098px;
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
</style>
