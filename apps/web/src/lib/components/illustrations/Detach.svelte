<!--
  The landing page's hero: one 18 s loop, driven by a script timeline so
  every frame is deterministic (and seekable for review: window.__heroSeek).
  A real laptop in 3D types `repose run`, gets the CLI's actual output and a
  tmux session with Claude Code working; the lid closes on its hinge; the
  project's machine keeps working through a time-lapse night; the lid opens
  and `repose attach izma` lands in the same session with the work done.
  Reduced motion shows the last frame.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	const T = 18000;

	// ---- the script -------------------------------------------------------
	const RUN = 'repose run "fix the flaky auth tests"';
	const ATTACH = 'repose attach izma';
	const TYPE_RUN = [700, 45]; // start, ms per char
	const TYPE_ATTACH = [13900, 55];

	type Line = { at: number; text: string; cls?: string };
	const cliLines: Line[] = [
		{ at: 2750, text: '✓ Created izma (large)            0.1s', cls: 'ok' },
		{ at: 3150, text: '✓ Built the environment           5.3s', cls: 'ok' },
		{ at: 3750, text: '✓ Booted izma                      11s', cls: 'ok' },
		{ at: 4050, text: 'Synced: 4 modified, 2 untracked (3 new commits)' },
		{ at: 4300, text: 'Ready in 15s.' }
	];
	// Claude Code's own shape: ⏺ tool call, ⎿ its result.
	const agentLines: Line[] = [
		{ at: 4900, text: '> fix the flaky auth tests', cls: 'prompt' },
		{ at: 5400, text: '⏺ Read(src/auth/session.ts)', cls: 'call' },
		{ at: 5700, text: '  ⎿  Read 214 lines', cls: 'dim' },
		{ at: 6100, text: '⏺ Bash(pnpm test auth)', cls: 'call' },
		{ at: 6500, text: '  ⎿  2 failed, 140 passed', cls: 'bad' },
		{ at: 8200, text: '⏺ Update(src/auth/session.ts)', cls: 'call' },
		{ at: 8500, text: '  ⎿  12 additions, 4 removals', cls: 'dim' },
		{ at: 9300, text: '⏺ Bash(pnpm test auth)', cls: 'call' },
		{ at: 9800, text: '  ⎿  1 failed, 141 passed', cls: 'bad' },
		{ at: 10500, text: '⏺ Update(src/auth/refresh.ts)', cls: 'call' },
		{ at: 10800, text: '  ⎿  6 additions, 1 removal', cls: 'dim' },
		{ at: 11500, text: '⏺ Bash(pnpm test auth)', cls: 'call' },
		{ at: 11900, text: '  ⎿  142 passed', cls: 'good' },
		{ at: 12400, text: '⏺ Both tests raced the token refresh.', cls: 'say' },
		{ at: 12450, text: '  Retries now wait for it. 142 pass.', cls: 'say' }
	];
	const TMUX_AT = 4700; // the CLI hands the terminal to tmux
	const LID_CLOSE = [6800, 8000]; // start, end
	const LID_OPEN = [12900, 13800];
	const ATTACHED_AT = 15200; // attach finished; laptop shows the session again

	// The machine's clock: 18:41 while you watch, a time-lapse night while
	// the lid is shut, 08:05 when it opens.
	function clock(t: number): string {
		const start = 18 * 60 + 41;
		const end = 24 * 60 + 8 * 60 + 5;
		let m: number;
		if (t < LID_CLOSE[1]) m = start;
		else if (t < LID_OPEN[0])
			m = start + 1 + ((t - LID_CLOSE[1]) / (LID_OPEN[0] - LID_CLOSE[1])) * (end - start - 1);
		else m = end;
		m = Math.floor(m) % (24 * 60);
		return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
	}

	function typed(text: string, [start, per]: number[], t: number): string {
		if (t < start) return '';
		return text.slice(0, Math.min(text.length, Math.floor((t - start) / per)));
	}

	const ease = (x: number) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2);
	function between(t: number, [a, b]: number[]) {
		return Math.min(1, Math.max(0, (t - a) / (b - a)));
	}

	// ---- time ---------------------------------------------------------------
	let t = $state(T - 1);
	let scale = $state(1);
	let wrap: HTMLDivElement;

	onMount(() => {
		const ro = new ResizeObserver(([e]) => (scale = e.contentRect.width / 760));
		ro.observe(wrap);
		const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		let seeking = false;
		(window as unknown as { __heroSeek?: (ms: number) => void }).__heroSeek = (ms) => {
			seeking = true;
			t = ms;
		};
		if (reduce) {
			t = T - 1;
			return () => ro.disconnect();
		}
		let raf = 0;
		const t0 = performance.now();
		const tick = (now: number) => {
			if (!seeking) t = (now - t0) % T;
			raf = requestAnimationFrame(tick);
		};
		raf = requestAnimationFrame(tick);
		return () => {
			cancelAnimationFrame(raf);
			ro.disconnect();
		};
	});

	// ---- derived frame ------------------------------------------------------
	// Lid angle: 12° past upright when open, -89° when shut: resting just
	// above the deck (at -92° it is coplanar with it and loses the depth test).
	let lid = $derived.by(() => {
		const c = ease(between(t, LID_CLOSE));
		const o = ease(between(t, LID_OPEN));
		return 12 - 101 * c + 101 * o;
	});
	let closed = $derived(t >= LID_CLOSE[1] && t < LID_OPEN[0]);
	let screenOn = $derived(Math.max(0, Math.min(1, (lid + 82) / 22)));
	let laptopView = $derived.by(() => {
		if (t < TMUX_AT) return 'cli';
		if (t < LID_OPEN[0]) return 'tmux';
		if (t < ATTACHED_AT) return 'attach';
		return 'tmux';
	});
	let runTyped = $derived(typed(RUN, TYPE_RUN, t));
	let attachTyped = $derived(typed(ATTACH, TYPE_ATTACH, t));
	let cliShown = $derived(cliLines.filter((l) => t >= l.at));
	let agentShown = $derived(t >= TMUX_AT ? agentLines.filter((l) => t >= l.at) : []);
	// What the laptop last saw before the lid closed stays on its screen.
	let laptopAgent = $derived(
		laptopView === 'tmux' && t < LID_OPEN[0]
			? agentLines.filter((l) => l.at <= Math.min(t, LID_CLOSE[0]))
			: agentShown
	);
	let linkLive = $derived(t < LID_CLOSE[0] + 300 || t >= ATTACHED_AT);
	let now = $derived(clock(t));
	let caretOn = $derived(Math.floor(t / 530) % 2 === 0);
	let working = $derived(t >= TMUX_AT && t < 12400);
	let spinner = $derived('✻✽✶✳✢·'[Math.floor(t / 120) % 6]);
	let loopFade = $derived(t > T - 250 ? (T - t) / 250 : t < 200 ? t / 200 : 1);
</script>

{#snippet tmuxScreen(lines: Line[], busy: boolean, time: string)}
	<div class="term-body">
		{#each lines.slice(-11) as l (l.at)}
			<div class="ln {l.cls ?? ''}">{l.text}</div>
		{/each}
		{#if busy}
			<div class="ln think">{spinner} Working…</div>
		{/if}
	</div>
	<div class="tmux-bar">
		<span>[izma] 0:claude*</span>
		<span>"repose-guest" {time} 23-Sep-26</span>
	</div>
{/snippet}

<div
	bind:this={wrap}
	class="hero-anim relative w-full"
	style="aspect-ratio: 760 / 380"
	role="img"
	aria-label="A laptop runs repose run and hands a task to Claude Code on the project's machine, then closes its lid. The machine keeps working overnight until the tests pass. In the morning the laptop opens, runs repose attach, and is back in the same session."
>
	<div class="stage" style="transform: scale({scale}); --clear: {loopFade}" aria-hidden="true">
		<!-- ================= the laptop ================= -->
		<div class="laptop">
			<div class="lid" style="transform: rotateX({lid}deg)">
				<div class="lid-front">
					<div class="screen">
						<div class="screen-inner" style="opacity: {screenOn}">
							{#if laptopView === 'cli'}
								<div class="term-body">
									<div class="ln dim">~/code/izma on main</div>
									<div class="ln">
										<span class="pr">❯</span>
										{runTyped}{#if cliShown.length === 0 && caretOn}<span class="caret"></span>{/if}
									</div>
									{#each cliShown as l (l.at)}
										<div class="ln {l.cls ?? ''}">{l.text}</div>
									{/each}
								</div>
							{:else if laptopView === 'attach'}
								<div class="term-body">
									<div class="ln dim">client_loop: send disconnect: Broken pipe</div>
									<div class="ln"></div>
									<div class="ln dim">~/code/izma on main</div>
									<div class="ln">
										<span class="pr">❯</span>
										{attachTyped}{#if caretOn}<span class="caret"></span>{/if}
									</div>
								</div>
							{:else}
								{@render tmuxScreen(laptopAgent, working && !closed, now)}
							{/if}
						</div>
						<div class="glare"></div>
					</div>
					<div class="camera"></div>
				</div>
				<div class="lid-back"></div>
			</div>
			<div class="deck">
				<div class="keys">
					{#each Array.from({ length: 60 }, (_, i) => i) as i (i)}
						<span class:wide={i === 59}></span>
					{/each}
				</div>
				<div class="pad"></div>
			</div>
			<div class="front-edge"></div>
			<div class="caption left">your laptop</div>
		</div>

		<!-- ================= the link ================= -->
		<svg class="wire-svg" viewBox="0 0 760 380" width="760" height="380">
			<path d="M398 250 C 430 250 432 160 466 160" class="wire" />
			{#if linkLive}
				<path d="M398 250 C 430 250 432 160 466 160" class="wire live" />
			{/if}
			<text x="436" y="276" class="wire-label" text-anchor="middle"
				>{linkLive ? 'ssh' : 'detached'}</text
			>
		</svg>

		<!-- ================= the machine ================= -->
		<div class="machine">
			<div class="win-bar">
				<span class="run"></span>
				<span class="win-title">izma</span>
				<span class="win-sub">large · host-01</span>
				<span class="win-clock">{now}</span>
			</div>
			<div class="screen machine-screen">
				<div class="screen-inner">
					{#if t < TMUX_AT}
						<div class="term-body">
							<div class="ln dim">booting…</div>
							{#if t > 3750}<div class="ln dim">sshd listening</div>{/if}
						</div>
					{:else}
						{@render tmuxScreen(agentShown, working, now)}
					{/if}
				</div>
			</div>
			<div class="caption right">
				the project’s machine{#if closed}<span class="still">, still working</span>{/if}
			</div>
		</div>
	</div>
</div>

<style>
	.hero-anim {
		--alu: #d9d9d6;
		--alu-2: #c7c7c3;
		--alu-3: #b5b5b0;
		--alu-edge: #a3a39e;
		--ink: #181817;
		--mute: #767671;
		--rule: #cfcfcb;
		overflow: hidden;
	}
	@media (prefers-color-scheme: dark) {
		.hero-anim {
			--alu: #3a3a38;
			--alu-2: #313130;
			--alu-3: #2a2a28;
			--alu-edge: #222221;
			--ink: #f3f3f1;
			--mute: #a2a29d;
			--rule: #3b3b38;
		}
	}
	.stage {
		position: absolute;
		left: 0;
		top: 0;
		width: 760px;
		height: 380px;
		transform-origin: 0 0;
		font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
		color: var(--ink);
	}

	/* ---------------- laptop ---------------- */
	.laptop {
		position: absolute;
		left: 46px;
		top: 20px;
		width: 352px;
		height: 330px;
		perspective: 1100px;
		perspective-origin: 50% -30%;
		transform-style: preserve-3d;
	}
	.lid {
		position: absolute;
		left: 11px;
		top: 18px;
		width: 330px;
		height: 214px;
		transform-origin: 50% 100%;
		transform-style: preserve-3d;
	}
	.lid-front,
	.lid-back {
		position: absolute;
		inset: 0;
		backface-visibility: hidden;
	}
	.lid-front {
		background: #0e0e0e;
		border-radius: 7px 7px 0 0;
		padding: 11px 11px 14px;
		box-shadow: inset 0 0 0 1px #2c2c2a;
	}
	.lid-back {
		transform: rotateX(180deg);
		background: linear-gradient(180deg, var(--alu) 0%, var(--alu-2) 100%);
		border-radius: 7px 7px 0 0;
		box-shadow: inset 0 0 0 1px var(--alu-edge);
	}
	.camera {
		position: absolute;
		top: 4px;
		left: 50%;
		width: 3px;
		height: 3px;
		margin-left: -1.5px;
		border-radius: 50%;
		background: #2a2a28;
	}
	.screen {
		position: relative;
		width: 100%;
		height: 100%;
		background: #000;
		overflow: hidden;
	}
	.screen-inner {
		/* between loops only the screens clear, like a terminal's clear */
		filter: opacity(var(--clear, 1));
		position: absolute;
		inset: 0;
		background: #0c0c0c;
		display: flex;
		flex-direction: column;
	}
	.glare {
		position: absolute;
		inset: 0;
		pointer-events: none;
		background: linear-gradient(115deg, rgb(255 255 255 / 0.05) 0%, transparent 38%);
	}
	.deck {
		position: absolute;
		left: 0;
		top: 232px;
		width: 352px;
		height: 196px;
		transform-origin: 50% 0;
		transform: rotateX(88deg);
		background: linear-gradient(180deg, var(--alu-2), var(--alu));
		border-radius: 0 0 9px 9px;
		box-shadow: inset 0 0 0 1px var(--alu-edge);
	}
	.keys {
		position: absolute;
		left: 26px;
		right: 26px;
		top: 18px;
		display: grid;
		grid-template-columns: repeat(12, 1fr);
		gap: 4px;
	}
	.keys span {
		height: 17px;
		border-radius: 2px;
		background: #1c1c1b;
	}
	.keys span.wide {
		grid-column: 4 / span 6;
	}
	.pad {
		position: absolute;
		left: 50%;
		top: 128px;
		width: 118px;
		height: 56px;
		margin-left: -59px;
		border-radius: 4px;
		background: var(--alu-3);
		box-shadow: inset 0 0 0 1px var(--alu-edge);
	}
	.front-edge {
		position: absolute;
		left: -2px;
		top: 236px;
		width: 356px;
		height: 7px;
		background: linear-gradient(180deg, var(--alu-3), var(--alu-edge));
		border-radius: 0 0 5px 5px;
	}

	/* ---------------- terminal text (both screens) ---------------- */
	.term-body {
		flex: 1;
		padding: 7px 8px 4px;
		font-size: 8.6px;
		line-height: 1.55;
		color: #d8d8d3;
		white-space: pre;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		justify-content: flex-start;
	}
	.ln {
		min-height: 1.55em;
	}
	.ln.dim {
		color: #8a8a85;
	}
	.ln.ok {
		color: #d8d8d3;
	}
	.ln.ok::first-letter {
		color: #5fb37f;
	}
	.pr {
		color: #b58cf0;
	}
	.ln.prompt {
		color: #a9a9a4;
	}
	.ln.call {
		color: #e8e8e3;
	}
	.ln.call::first-letter {
		color: #5fb37f;
	}
	.ln.bad {
		color: #e07a6e;
	}
	.ln.good {
		color: #7fce9d;
	}
	.ln.say {
		color: #e8e8e3;
	}
	.ln.think {
		color: #d99a5b;
	}
	.caret {
		display: inline-block;
		width: 0.6em;
		height: 1.15em;
		margin-left: 1px;
		vertical-align: -0.2em;
		background: #d8d8d3;
	}
	.tmux-bar {
		display: flex;
		justify-content: space-between;
		padding: 0 6px;
		font-size: 8.2px;
		line-height: 13px;
		background: #1f9d55;
		color: #0b0b0b;
		white-space: pre;
	}

	/* ---------------- link ---------------- */
	.wire-svg {
		position: absolute;
		left: 0;
		top: 0;
		overflow: visible;
		pointer-events: none;
	}
	.wire {
		fill: none;
		stroke: var(--mute);
		stroke-width: 1;
		stroke-dasharray: 2 4;
		opacity: 0.6;
	}
	.wire.live {
		stroke: var(--ink);
		stroke-width: 1.4;
		stroke-dasharray: 8 6;
		opacity: 1;
		animation: flow 0.9s linear infinite;
	}
	@keyframes flow {
		to {
			stroke-dashoffset: -28;
		}
	}
	.wire-label {
		font-size: 10px;
		fill: var(--mute);
	}

	/* ---------------- machine ---------------- */
	.machine {
		position: absolute;
		left: 466px;
		top: 40px;
		width: 280px;
		height: 250px;
		border: 1px solid var(--rule);
		background: #0c0c0c;
		display: flex;
		flex-direction: column;
	}
	.win-bar {
		display: flex;
		align-items: center;
		gap: 7px;
		height: 24px;
		padding: 0 9px;
		font-size: 9.5px;
		background: var(--alu);
		color: var(--ink);
		border-bottom: 1px solid var(--rule);
	}
	.run {
		width: 7px;
		height: 7px;
		background: #3f8a5e;
	}
	.win-title {
		font-weight: 600;
	}
	.win-sub {
		color: var(--mute);
	}
	.win-clock {
		margin-left: auto;
		color: var(--mute);
		font-variant-numeric: tabular-nums;
	}
	.machine-screen {
		flex: 1;
		height: auto;
	}
	.caption {
		position: absolute;
		font-size: 10px;
		color: var(--mute);
		white-space: nowrap;
	}
	.caption.left {
		left: 0;
		right: 0;
		top: 318px;
		text-align: center;
	}
	.caption.right {
		left: 0;
		top: 262px;
	}
	.still {
		color: var(--ink);
	}
</style>
