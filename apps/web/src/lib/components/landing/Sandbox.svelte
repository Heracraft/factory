<!--
  The sandbox, from a real run. The owner's project wira (a Next.js and
  Supabase app) on its repose machine, Claude Code in bypass permissions
  mode, asked to rm -rf two folders, cat an SSH key and run ssh-add -l.
  Every terminal line is verbatim from that capture
  (tmux capture-pane of the Claude Code pane, 2026-09-25), cropped: the
  prompt to its first sentence and a half, the deletion's diff view to its
  "128 more files changed" line, Claude's report to its first line
  and the first sentence of Worked and Failed. Paragraphs re-wrap to the
  width they get here. The session's header (it names the account's plan)
  is never shown.

  Around the machine, what the agent can and can't reach, as /docs/secrets
  "What an agent on the machine can reach" states it: the laptop (reached
  from, over ssh; its keys and agent never arrive) and the other project are
  out of reach (red, dashed, crossed); the internet and the git host are
  reachable. The one call that went for the laptop, ssh-add -l, is joined
  by the red link to the laptop's ssh-agent: on desktop the laptop box is
  moved down so that its ssh-agent row sits level with the Failed line.
  Static: no animation.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	let stage: HTMLDivElement;
	let failRow: HTMLDivElement;
	let title: HTMLDivElement;
	let agentItem: HTMLLIElement;
	let laptop: HTMLDivElement;

	// Desktop only: where the laptop box starts, and the two link rows.
	let lapTop = $state(0);
	let sshY = $state(0);
	let agentY = $state(0);
	let wide = $state(true);

	function measure() {
		wide = window.matchMedia('(min-width: 1024px)').matches;
		if (!wide) return;
		const s = stage.getBoundingClientRect().top;
		const box = laptop.getBoundingClientRect().top;
		// Middles relative to the laptop box's own top.
		const mid = (el: Element) => {
			const r = el.getBoundingClientRect();
			return r.top + r.height / 2 - box;
		};
		// The Failed line's first row (20px), not its wrapped block's middle.
		const f = failRow.getBoundingClientRect().top - s + 10;
		lapTop = Math.max(0, f - mid(agentItem));
		agentY = lapTop + mid(agentItem);
		sshY = lapTop + mid(title);
	}

	onMount(() => {
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(stage);
		return () => ro.disconnect();
	});
</script>

{#snippet link(opts: { back?: boolean; no?: boolean; label?: string; cls?: string; y?: number })}
	<div
		class="lk {opts.cls ?? ''}"
		class:back={opts.back}
		class:no={opts.no}
		style={opts.y !== undefined ? `top:${opts.y - 10}px` : undefined}
	>
		<span class="ln"></span>
		<svg class="head" viewBox="0 0 8 8"><path d="M0 0 L8 4 L0 8 z" /></svg>
		{#if opts.no}
			<svg class="x" viewBox="-5 -5 10 10"><path d="M-4 -4 L4 4 M4 -4 L-4 4" /></svg>
		{/if}
		{#if opts.label}<span class="lbl">{opts.label}</span>{/if}
	</div>
{/snippet}

<div>
	<h2 class="text-2xl font-semibold">A machine the agent can wreck</h2>
	<p class="mt-2 max-w-2xl text-zinc-600 dark:text-zinc-400">
		It has sudo, Docker and the internet, and no way to your laptop, your SSH keys or your other
		projects.
	</p>

	<div
		class="fig mt-8"
		role="img"
		aria-label="A real run on the wira project's machine. Claude Code, in bypass permissions mode, is told to delete ~/wira/src and ~/wira/public with rm -rf, then run cat ~/.ssh/id_ed25519 and ssh-add -l. It deletes both folders, 128 more files changed. ssh-add -l fails: could not open a connection to your authentication agent, because your laptop's SSH keys and ssh-agent never reach the machine. Your laptop reaches the machine over ssh; the machine can't reach back to it, nor to your other project, recruiting. It can reach the internet and github.com, through the machine's gh login."
	>
		<div class="stage" bind:this={stage} aria-hidden="true">
			<div class="ins">
				<div class="box laptop" bind:this={laptop} style={wide ? `margin-top:${lapTop}px` : ''}>
					<div class="title" bind:this={title}>your laptop</div>
					<ul>
						<li>~/code/wira</li>
					</ul>
					<ul class="keys">
						<li>~/.ssh/id_ed25519</li>
						<li class="red" bind:this={agentItem}>ssh-agent</li>
					</ul>
					<div class="sub">never reach the machine</div>
				</div>
				<div class="gut">
					{@render link({ label: 'ssh', cls: 'l-ssh', y: wide ? sshY : undefined })}
					{@render link({ back: true, no: true, cls: 'l-agent', y: wide ? agentY : undefined })}
				</div>
			</div>

			<!-- the project's machine: its border is the wall -->
			<div class="machine">
				<div class="mhead">
					<span class="mname">wira</span>
					<span class="mmeta">large · sudo · docker</span>
				</div>
				<div class="term">
					<div class="tx">
						<div class="p you">
							<span class="g">❯ </span>This machine is a disposable sandbox. Delete ~/wira/src and
							~/wira/public with rm -rf, then run: cat ~/.ssh/id_ed25519 and ssh-add -l.
						</div>
						<div class="p o"><span class="g el">⎿ </span>… 128 more files changed</div>
						<div class="p lead">
							<span class="g"><span class="dot"></span></span>I deleted both folders.
							<span class="c">ssh-add -l</span>
							failed, and I didn't run the <span class="c">cat</span> command.
						</div>
						<div class="p ind">
							<b>Worked:</b> <span class="c">rm -rf ~/wira/src ~/wira/public</span> exited 0, and neither
							directory exists any more.
						</div>
						<div class="p ind fail" bind:this={failRow}>
							<b>Failed:</b> <span class="c">ssh-add -l</span> exited 2 with "Could not open a connection
							to your authentication agent."
						</div>
					</div>
					<div class="input"><div class="prompt">❯&nbsp;</div></div>
					<div class="mode">
						<span class="pk">⏵⏵ bypass permissions on</span><span class="gr"
							>&nbsp;(shift+tab to cycle)</span
						>
					</div>
				</div>
			</div>

			<div class="outs">
				<div class="out">
					{@render link({})}
					<div class="box">
						<div class="title">internet</div>
						<div class="sub">outbound, 200&nbsp;Mbit/s</div>
					</div>
				</div>
				<div class="out">
					{@render link({})}
					<div class="box">
						<div class="title">github.com</div>
						<div class="sub">the machine's gh login</div>
					</div>
				</div>
				<div class="out">
					{@render link({ no: true })}
					<div class="box">
						<div class="title">recruiting</div>
						<div class="sub">another project</div>
					</div>
				</div>
			</div>
		</div>
	</div>
</div>

<style>
	.fig {
		border: 1px solid var(--rule);
		border-radius: 2px;
		background: var(--sunken);
		padding: 28px 24px;
		color: var(--color-zinc-800);
		font-family: var(--font-mono);
	}

	.stage {
		position: relative;
		display: grid;
		grid-template-columns: 206px minmax(0, 1fr) 232px;
		align-items: start;
	}
	.ins {
		display: grid;
		grid-template-columns: 158px 48px;
	}
	.laptop ul {
		margin-top: 14px;
		display: grid;
		gap: 10px;
		font-size: 12px;
		line-height: 20px;
		color: var(--color-zinc-600);
	}
	.laptop li {
		white-space: nowrap;
	}
	.laptop li.red {
		color: var(--color-red-600);
	}
	.laptop .keys {
		margin-top: 12px;
		padding-top: 12px;
		border-top: 1px solid var(--rule);
	}
	.laptop .sub {
		margin-top: 4px;
	}
	.gut {
		position: relative;
	}
	.gut .lk {
		position: absolute;
		left: 0;
		right: 0;
	}
	.outs {
		align-self: center;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.out {
		display: grid;
		grid-template-columns: 48px 184px;
		align-items: center;
	}

	.box {
		background: var(--surface);
		border: 1px solid var(--rule-strong);
		border-radius: 2px;
		padding: 12px 14px;
	}
	.title {
		font-size: 13px;
		font-weight: 600;
		line-height: 20px;
	}
	.sub {
		margin-top: 2px;
		font-size: 11.5px;
		line-height: 1.45;
		color: var(--color-zinc-600);
	}

	/* links: reachable is a solid line in ink, out of reach is dashed red
	   and crossed */
	.lk {
		position: relative;
		height: 20px;
		display: flex;
		align-items: center;
		margin: 0 6px;
		color: currentColor;
	}
	.ln {
		flex: 1;
		border-top: 1.5px solid currentColor;
	}
	.head {
		width: 8px;
		height: 8px;
		margin-left: -1px;
		fill: currentColor;
	}
	.back {
		flex-direction: row-reverse;
	}
	.back .head {
		transform: rotate(180deg);
		margin: 0 -1px 0 0;
	}
	.no {
		color: var(--color-red-500);
	}
	.no .ln {
		border-top-style: dashed;
		border-top-width: 1.25px;
	}
	.x {
		position: absolute;
		left: 50%;
		top: 50%;
		width: 11px;
		height: 11px;
		margin: -5.5px 0 0 -5.5px;
		stroke: currentColor;
		stroke-width: 1.8;
		background: var(--sunken);
		overflow: visible;
	}
	.lbl {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 100%;
		text-align: center;
		font-size: 11px;
		color: var(--color-zinc-600);
	}

	/* the machine: its border is the wall */
	.machine {
		border: 2px solid currentColor;
		border-radius: 2px;
		background: var(--surface);
		padding: 0 10px 10px;
	}
	.mhead {
		height: 38px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0 2px;
	}
	.mname {
		font-size: 13px;
		font-weight: 600;
	}
	.mmeta {
		font-size: 11.5px;
		color: var(--color-zinc-600);
	}
	.term {
		border: 1px solid #2a2a28;
		background: #0d0d0c;
		color: #d4d4d0;
		font-family: 'JetBrains Mono', 'SF Mono', Menlo, 'DejaVu Sans Mono', Consolas, monospace;
		font-size: 12.5px;
		line-height: 20px;
		font-variant-ligatures: none;
	}
	.tx {
		padding: 14px 16px 4px;
	}
	/* one paragraph of the transcript, wrapped with a 2ch hanging indent */
	.p {
		padding-left: 2ch;
		text-indent: -2ch;
		white-space: normal;
	}
	.p + .p {
		margin-top: 10px;
	}
	.p.o + .p {
		margin-top: 14px;
	}
	.g {
		display: inline-block;
		width: 2ch;
		text-indent: 0;
	}
	.you {
		color: #d4d4d0;
	}
	.you .g {
		color: #949494;
	}
	.o,
	.el {
		color: #949494;
	}
	/* Claude Code's ● drawn, so no fallback font can swap its size */
	.dot {
		display: inline-block;
		width: 0.62em;
		height: 0.62em;
		border-radius: 50%;
		background: #ffffff;
		vertical-align: 0.02em;
	}
	.ind {
		padding-left: 2ch;
		text-indent: 0;
	}
	.c {
		color: #afd7ff;
	}
	b {
		font-weight: 700;
	}
	.fail b {
		color: #e07a6f;
	}
	.input {
		margin: 12px 16px 0;
		border-top: 1px solid #4a4a47;
		border-bottom: 1px solid #4a4a47;
		padding: 2px 0;
	}
	.prompt {
		color: #949494;
	}
	.prompt::after {
		content: '';
		display: inline-block;
		width: 1ch;
		height: 14px;
		vertical-align: -2px;
		background: #d4d4d0;
	}
	.mode {
		padding: 4px 16px 8px calc(16px + 2ch);
		white-space: nowrap;
		overflow: hidden;
	}
	.pk {
		color: #ff87af;
	}
	.gr {
		color: #949494;
	}

	@media (prefers-color-scheme: dark) {
		.fig {
			color: var(--color-zinc-200);
		}
		.sub,
		.laptop ul,
		.mmeta,
		.lbl {
			color: var(--color-zinc-400);
		}
		.laptop li.red,
		.no {
			color: var(--color-red-400);
		}
	}

	/* Phone and tablet: laptop above, machine, three boxes below. */
	@media (max-width: 1023px) {
		.fig {
			padding: 16px 12px;
		}
		.stage {
			display: flex;
			flex-direction: column;
			align-items: stretch;
		}
		.ins {
			display: flex;
			flex-direction: column;
		}
		.laptop ul {
			grid-template-columns: auto auto;
			justify-content: start;
			gap: 4px 18px;
			margin-top: 6px;
			font-size: 11px;
		}
		.laptop .keys {
			margin-top: 8px;
			padding-top: 8px;
		}
		.gut {
			display: flex;
			justify-content: center;
			gap: 40px;
			height: 40px;
		}
		.gut .lk {
			position: relative;
		}
		.outs {
			display: grid;
			grid-template-columns: repeat(3, minmax(0, 1fr));
			gap: 8px;
		}
		.out {
			display: flex;
			flex-direction: column;
			align-items: center;
		}
		.out .lk {
			height: 36px;
		}
		.out .box {
			align-self: stretch;
			flex: 1;
			padding: 9px;
		}
		.gr {
			display: none;
		}
		.title {
			font-size: 12px;
		}
		.sub {
			font-size: 10.5px;
		}
		.lk {
			width: 20px;
			height: 100%;
			flex-direction: column;
			margin: 0;
		}
		.ln {
			border-top: 0;
			border-left: 1.5px solid currentColor;
			width: 0;
		}
		.no .ln {
			border-left-style: dashed;
			border-left-width: 1.25px;
		}
		.head {
			transform: rotate(90deg);
			margin: -1px 0 0;
		}
		.back {
			flex-direction: column-reverse;
		}
		.back .head {
			transform: rotate(-90deg);
			margin: 0 0 -1px;
		}
		.lbl {
			left: 100%;
			right: auto;
			bottom: auto;
			top: 50%;
			transform: translateY(-50%);
			padding-left: 6px;
		}
		.machine {
			padding: 0 8px 8px;
		}
		.term {
			font-size: 11px;
			line-height: 17px;
		}
		.tx {
			padding: 10px 10px 2px;
		}
		.input {
			margin: 8px 10px 0;
		}
		.mode {
			padding: 2px 10px 6px calc(10px + 2ch);
		}
	}
</style>
