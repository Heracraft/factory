<!--
  Ready: what is on a machine the moment it boots. The five agents are the
  hero (mark, name as the docs spell it, and the command that starts it);
  below them the toolchain, and a shell on the machine where a missing
  command prints the real command-not-found hint (nix/guest/base/devtools.nix,
  captured for real on the recruiting machine).
-->
<script lang="ts">
	import { agentMarks, toolMarks, type Mark } from '$lib/components/illustrations/marks';

	// Names as apps/web/src/content/docs/agents.md spells them, and the
	// command each one starts with (its `--agent` value there).
	const agentInfo: Record<string, { name: string; cmd: string }> = {
		'Claude Code': { name: 'Claude Code', cmd: 'claude' },
		Codex: { name: 'Codex CLI', cmd: 'codex' },
		opencode: { name: 'opencode', cmd: 'opencode' },
		'Gemini CLI': { name: 'Gemini CLI', cmd: 'gemini' },
		pi: { name: 'pi', cmd: 'pi' }
	};

	// Labels from docs/machine.md "What's installed".
	const toolLabel: Record<string, string> = {
		'Node.js': 'Node.js 24',
		Python: 'Python 3.12',
		Rust: 'rustup'
	};

	const everyday = 'uv gcc make cmake git gh tmux jq ripgrep psql neovim Chromium';

	// A real capture from the `recruiting` machine (tmux capture-pane -p -e -J,
	// 2026-09-25): pgcli typed in the checkout, the command-not-found hint, the
	// suggested install, the tool running. Prompt rows are cropped after the
	// directory (the rest is Nerd Font glyphs); colours as the capture set them,
	// mapped as ops/dev/hero/convert.py maps them.
	type Row = [string, string][];
	const prompt: Row = [
		['dev', 'y b'],
		[' in ', ''],
		['🌐 repose-guest', 'g b dim'],
		[' in ', ''],
		['recruiting', 'c b']
	];
	const rows: (Row | 'hint')[] = [
		prompt,
		[
			['❯', 'g b'],
			[' pgcli -p 5433', '']
		],
		[['pgcli: command not found', '']],
		'hint',
		[['Other packages with pgcli: python314Packages.pgcli, python313Packages.pgcli', '']],
		[],
		prompt,
		[
			['❯', 'r'],
			[' nix profile add nixpkgs#pgcli', '']
		],
		[],
		prompt,
		[
			['❯', 'g b'],
			[' pgcli --version', '']
		],
		[['Version: 4.6.0', '']]
	];
	// The status bar as a client on that window drew it (the mode flag cropped).
	const barHost = '"repose-guest" ';
	const barDate = ' 25-Sep-26';
	const hint = [
		['  nix profile add nixpkgs#pgcli', 'install it on this machine'],
		['  repose config add pgcli      ', 'keep it on every rebuild (run this on your laptop)']
	];
</script>

{#snippet mark(m: Mark, size: number)}
	<svg
		viewBox="0 0 24 24"
		width={size}
		height={size}
		class="shrink-0"
		fill="currentColor"
		aria-hidden="true"
		>{#each m.paths as d (d)}<path
				{d}
				fill-rule={m.evenodd ? 'evenodd' : 'nonzero'}
				clip-rule={m.evenodd ? 'evenodd' : 'nonzero'}
			/>{/each}</svg
	>
{/snippet}

<h2 class="text-2xl font-semibold">Five agents and a full toolchain on first boot</h2>
<p class="mt-2 max-w-2xl text-zinc-600 dark:text-zinc-400">
	The agent has sudo to install anything else, and
	<code
		class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
		>repose config add</code
	> keeps it on every rebuild.
</p>

<div
	class="ready mt-8 rounded-xs border border-[var(--rule)] bg-[var(--sunken)]"
	role="img"
	aria-label="A new machine has five coding agents installed: Claude Code, Codex CLI, opencode, Gemini CLI and pi. Also Node.js 24, pnpm, Python 3.12, Go, rustup, Docker, Nix, Chromium and everyday tools. A shell on the machine: typing pgcli, which is not installed, prints how to install it with nix profile add nixpkgs#pgcli or keep it on every rebuild with repose config add pgcli; after the install, pgcli --version prints 4.6.0."
>
	<ul class="agents" aria-hidden="true">
		{#each agentMarks as m (m.name)}
			<li>
				<span class="am">{@render mark(m, 40)}</span>
				<span class="an">{agentInfo[m.name].name}</span>
				<span class="ac">{agentInfo[m.name].cmd}</span>
			</li>
		{/each}
	</ul>

	<div class="below" aria-hidden="true">
		<div>
			<ul class="tools">
				{#each toolMarks as m (m.name)}
					<li>
						{@render mark(m, 18)}{toolLabel[m.name] ?? m.name}
					</li>
				{/each}
			</ul>
			<p class="more">
				{everyday} …
			</p>
		</div>

		<div class="term">
			<div class="lines">
				{#each rows as row, i (i)}
					{#if row === 'hint'}
						{#each hint as [cmd, what] (cmd)}
							<div class="hint">
								<span>{cmd}</span><span class="sp">&nbsp;&nbsp;</span><span class="d">{what}</span>
							</div>
						{/each}
					{:else}
						<div>
							{#each row as [text, cls], j (j)}<span class={cls}>{text}</span
								>{/each}{#if row.length === 0}&nbsp;{/if}
						</div>
					{/if}
				{/each}
			</div>
			<div class="bar">
				<span class="pre">[recruitin0:shell- 1:dev&nbsp; 2:ready*</span><span class="clock"
					><span class="long">{barHost}</span>19:49<span class="long">{barDate}</span></span
				>
			</div>
		</div>
	</div>
</div>

<style>
	.agents {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 28px 8px;
		padding: 32px 12px;
	}
	.agents li {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
	}
	.am {
		color: var(--color-zinc-900);
	}
	.an {
		margin-top: 12px;
		font-size: 14px;
		font-weight: 500;
		color: var(--color-zinc-800);
	}
	.ac {
		margin-top: 2px;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--color-zinc-500);
	}
	.below {
		display: grid;
		gap: 24px;
		padding: 16px;
		align-items: start;
		border-top: 1px solid var(--rule);
	}
	.tools {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px 16px;
	}
	.tools li {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 14px;
		color: var(--color-zinc-700);
	}
	.more {
		margin-top: 16px;
		font-family: var(--font-mono);
		font-size: 12px;
		line-height: 20px;
		color: var(--color-zinc-500);
	}
	@media (min-width: 640px) {
		.agents {
			grid-template-columns: repeat(5, minmax(0, 1fr));
			padding: 40px 32px;
		}
		.below {
			padding: 32px;
		}
	}
	@media (min-width: 768px) {
		.below {
			grid-template-columns: minmax(0, 15rem) minmax(0, 1fr);
			gap: 32px;
		}
	}
	@media (prefers-color-scheme: dark) {
		.am {
			color: var(--color-zinc-100);
		}
		.an {
			color: var(--color-zinc-200);
		}
		.ac,
		.more {
			color: var(--color-zinc-400);
		}
		.tools li {
			color: var(--color-zinc-300);
		}
	}

	.term {
		display: flex;
		flex-direction: column;
		min-width: 0;
		border: 1px solid #2a2a28;
		border-radius: 2px;
		background: #0d0d0c;
		color: #d4d4d0;
		font-family: var(--font-mono);
		font-size: 12px;
		line-height: 19px;
	}
	.lines {
		flex: 1;
		padding: 12px 14px 14px;
	}
	.lines > div {
		overflow-wrap: anywhere;
	}
	.lines span {
		white-space: pre-wrap;
	}
	.hint span {
		white-space: pre;
	}
	.y {
		color: #e5c07b;
	}
	.g {
		color: #98c379;
	}
	.c {
		color: #56b6c2;
	}
	.r {
		color: #e06c75;
	}
	.pre,
	.clock span {
		white-space: pre;
	}
	.b {
		font-weight: 700;
	}
	.dim {
		opacity: 0.7;
	}
	.bar {
		display: flex;
		justify-content: space-between;
		gap: 1ch;
		padding: 1px 14px;
		background: #1f9d55;
		color: #0d0d0c;
		white-space: nowrap;
		overflow: hidden;
	}
	@media (max-width: 639px) {
		.long {
			display: none;
		}
		.term {
			font-size: 10.5px;
			line-height: 16px;
		}
		.lines {
			padding: 10px 10px 12px;
		}
		.bar {
			padding: 1px 10px;
		}
		.hint {
			display: flex;
			flex-direction: column;
		}
		.hint .sp {
			display: none;
		}
		.hint .d {
			padding-left: 4ch;
			white-space: pre-wrap;
		}
		.agents {
			display: flex;
			flex-wrap: wrap;
			justify-content: center;
		}
		.agents li {
			width: calc((100% - 16px) / 3);
		}
	}
</style>
