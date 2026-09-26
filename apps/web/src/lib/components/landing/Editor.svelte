<!--
  Open it in your editor (docs run-and-attach.md "SSH and editors"). Not a
  drawing: LazyVim, installed on the real recruiting machine the documented
  way (the LazyVim starter cloned to ~/.config/nvim, plugins synced by
  lazy.nvim, stock config, tokyonight), in the job-alerts checkout on its
  fetch-timeout branch (the same state the "Your working state" section
  shows). The worker's poller.ts and config.ts are open, the cursor on the
  fetchTimeoutMs setting, and the snacks explorer on the left (opened with
  Snacks.explorer() and narrowed with :vertical resize 20, from a -c luafile
  so no stray keys show in lualine), so the tree stays shallow and the code
  gets most of the width. The 78x20 tmux pane was captured with
  `tmux capture-pane -p -e -N` and converted to styled runs verbatim; the
  block cursor is the terminal's own, at the cell tmux reported. No window
  chrome: it is the full screen, as you'd see it.

  Nerd Font icons come from a subset of Symbols Nerd Font Mono (Nerd Fonts,
  MIT; the icons are from Material Design Icons, Codicons, Octicons and
  Powerline), only the 9 glyphs this capture uses, served from
  static/landing/editor-nerd-symbols.woff2 and used by the capture alone.
  Every non-ASCII character sits in a one-cell box so the grid holds in any
  monospace font.
-->
<script lang="ts">
	// Verbatim capture: one inline style per distinct SGR state, and each row
	// as [text, style] runs.
	const styles: string[] = [
		'color:#c8d3f5;background:#1e2030;',
		'color:#636da6;background:#191b28;',
		'color:#0db9d7;background:#191b28;',
		'color:#12131d;background:#191b28;',
		'color:#7ca1f2;background:#1b1d2b;',
		'color:#c8d3f5;background:#222436;font-weight:700;font-style:italic;',
		'color:#0db9d7;background:#222436;',
		'color:#c8d3f5;background:#222436;',
		'color:#636da6;background:#12131d;',
		'color:#ff966c;background:#1e2030;',
		'color:#1b1d2b;background:#222436;font-weight:700;',
		'color:#3b4261;background:#222436;',
		'color:#86e1fc;background:#222436;',
		'color:#fca7ea;background:#222436;font-style:italic;',
		'color:#89ddff;background:#222436;',
		'color:#828bb8;background:#222436;',
		'color:#65bcff;background:#1e2030;',
		'color:#545c7e;background:#1e2030;',
		'color:#65bcff;background:#222436;',
		'color:#4fd6be;background:#222436;',
		'color:#82aaff;background:#222436;',
		'color:#c3e88d;background:#222436;',
		'color:#ff966c;background:#222436;',
		'background:#1e2030;',
		'color:#0db9d7;background:#1e2030;',
		'color:#82aaff;background:#1e2030;',
		'color:#3b4261;background:#1e2030;',
		'color:#fca7ea;background:#1e2030;',
		'color:#ffc777;background:#1e2030;',
		'color:#636da6;background:#222436;font-style:italic;',
		'color:#ff966c;background:#222436;font-weight:700;',
		'color:#65bcff;background:#2f334d;',
		'color:#c8d3f5;background:#2f334d;',
		'color:#4fd6be;background:#2f334d;',
		'color:#89ddff;background:#2f334d;',
		'color:#82aaff;background:#2f334d;',
		'color:#828bb8;background:#2f334d;',
		'color:#c3e88d;background:#2f334d;',
		'color:#ff966c;background:#2f334d;',
		'background:#2f334d;',
		'color:#3b4261;background:#2f334d;',
		'color:#0db9d7;background:#2f334d;',
		'color:#1b1d2b;background:#82aaff;',
		'color:#82aaff;background:#3b4261;',
		'color:#828bb8;background:#1e2030;',
		'color:#c8d3f5;background:#1e2030;font-weight:700;'
	];
	const rows: [string, number][][] = [
		[
			['                    ', 0],
			['    ', 1],
			['\udb81\udee6 ', 2],
			['poller.ts    \uea76 ', 1],
			['\u2595', 3],
			['\u258e', 4],
			['   ', 5],
			['\udb81\udee6 ', 6],
			['config.ts    ', 5],
			['\uea76 ', 7],
			['               ', 8]
		],
		[
			['\u256d\u2500\u2500\u2500\u2500 Explorer \u2500\u2500\u2500\u2500\u256e', 9],
			['\u2502', 10],
			['   9   ', 11],
			['export', 12],
			[' ', 7],
			['const', 13],
			[' config ', 7],
			['=', 14],
			[' ', 7],
			['{', 15],
			['                           ', 7]
		],
		[
			['\u2502', 9],
			['\uf105 ', 16],
			['          ', 0],
			['45/45', 17],
			[' ', 0],
			['\u2502', 9],
			['\u2502', 10],
			['   8   ', 11],
			['\u2502', 18],
			[' ', 7],
			['databaseUrl', 19],
			[':', 14],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['DATABASE_URL', 19],
			[',', 14],
			['          ', 7]
		],
		[
			[
				'\u2570\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u256f',
				9
			],
			['\u2502', 10],
			['   7   ', 11],
			['\u2502', 18],
			[' ', 7],
			['pollIntervalMin', 19],
			[':', 14],
			[' ', 7],
			['num', 20],
			['(', 15],
			['"POLL_INTERVAL_MIN"', 21],
			[',', 14],
			[' ', 7],
			['10', 22],
			[')', 15],
			[',', 14],
			['  ', 7]
		],
		[
			[' ', 23],
			['\udb81\udf70 ', 24],
			['recruiting', 25],
			['       ', 0],
			['\u2502', 10],
			['   6   ', 11],
			['\u2502', 18],
			[' ', 7],
			['userAgent', 19],
			[':', 14],
			['                                      ', 7]
		],
		[
			[' ', 23],
			['\u251c\u2574', 26],
			['\udb81\udf70 ', 24],
			['apps', 25],
			['           ', 0],
			['\u2502', 10],
			['   5   ', 11],
			['\u2502', 18],
			[' ', 7],
			['\u2502', 11],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['HTTP_USER_AGENT', 19],
			[' ', 7],
			['??', 14],
			['                ', 7]
		],
		[
			[' ', 23],
			['\u2502 \u251c\u2574', 26],
			['\udb80\ude4b ', 24],
			['web', 25],
			['          ', 0],
			['\u2502', 10],
			['   4   ', 11],
			['\u2502', 18],
			[' ', 7],
			['\u2502', 11],
			[' ', 7],
			['"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/5', 21]
		],
		[
			[' ', 0],
			['\u2502 \u2514\u2574', 26],
			['\udb81\udf70 ', 24],
			['worker', 25],
			['       ', 0],
			['\u2502', 10],
			['   3   ', 11],
			['\u2502', 18],
			[' ', 7],
			['fetchConcurrency', 19],
			[':', 14],
			[' ', 7],
			['num', 20],
			['(', 15],
			['"FETCH_CONCURRENCY"', 21],
			[',', 14],
			[' ', 7],
			['6', 22],
			[')', 15],
			[',', 14],
			['  ', 7]
		],
		[
			[' ', 23],
			['\u2502   \u251c\u2574', 26],
			['\udb81\udf70 ', 27],
			['src', 25],
			['        ', 0],
			['\u2502', 10],
			['   2   ', 11],
			['\u2502', 18],
			[' ', 7],
			['fetchPerHost', 19],
			[':', 14],
			[' ', 7],
			['num', 20],
			['(', 15],
			['"FETCH_PER_HOST"', 21],
			[',', 14],
			[' ', 7],
			['2', 22],
			[')', 15],
			[',', 14],
			['         ', 7]
		],
		[
			[' ', 23],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb80\ude4b ', 24],
			['ingest', 28],
			[' ', 0],
			['\u25cb', 28],
			[' ', 0],
			['\u2502', 10],
			['   1   ', 11],
			['\u2502', 18],
			[' ', 7],
			['/** Per-request timeout for board fetches, in mi', 29]
		],
		[
			[' ', 0],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb80\ude4b ', 24],
			['notify', 25],
			['   ', 0],
			['\u2502', 10],
			['  21   ', 30],
			['\u2502', 31],
			[' ', 32],
			['fetchTimeoutMs', 33],
			[':', 34],
			[' ', 32],
			['num', 35],
			['(', 36],
			['"FETCH_TIMEOUT_MS"', 37],
			[',', 34],
			[' ', 32],
			['20_000', 38],
			[')', 36],
			[',', 34]
		],
		[
			[' ', 0],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb80\ude4b ', 24],
			['ops', 25],
			['      ', 0],
			['\u2502', 10],
			['   1   ', 11],
			['\u2502', 18],
			['                                                 ', 7]
		],
		[
			[' ', 23],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb80\ude4b ', 24],
			['util', 25],
			['     ', 0],
			['\u2502', 10],
			['   2   ', 11],
			['\u2502', 18],
			[' ', 7],
			['resendApiKey', 19],
			[':', 14],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['RESEND_API_KEY', 19],
			[' ', 7],
			['??', 14],
			[' ', 7],
			['""', 21],
			[',', 14],
			[' ', 7]
		],
		[
			[' ', 39],
			['\u2502   \u2502 \u251c\u2574', 40],
			['\udb81\udee6 ', 41],
			['config.ts', 32],
			['\u2502', 10],
			['   3   ', 11],
			['\u2502', 18],
			[' ', 7],
			['emailFrom', 19],
			[':', 14],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['EMAIL_FROM', 19],
			[' ', 7],
			['??', 14],
			[' ', 7],
			['"alerts@exa', 21]
		],
		[
			[' ', 0],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb81\udee6 ', 24],
			['dashboard', 0],
			['\u2502', 10],
			['   4   ', 11],
			['\u2502', 18],
			[' ', 7],
			['emailFromName', 19],
			[':', 14],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['EMAIL_FROM_NAME', 19],
			[' ', 7],
			['??', 14],
			[' ', 7],
			['"J', 21]
		],
		[
			[' ', 0],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb81\udee6 ', 24],
			['digest-on', 0],
			['\u2502', 10],
			['   5   ', 11],
			['\u2502', 18],
			['                                                 ', 7]
		],
		[
			[' ', 23],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb81\udee6 ', 24],
			['education', 0],
			['\u2502', 10],
			['   6   ', 11],
			['\u2502', 18],
			[' ', 7],
			['operatorGchatWebhook', 19],
			[':', 14],
			[' process', 7],
			['.', 14],
			['env', 19],
			['.', 14],
			['OPERATOR_GCHAT', 19]
		],
		[
			[' ', 0],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb81\udee6 ', 24],
			['education', 0],
			['\u2502', 10],
			['   7   ', 11],
			['\u2502', 18],
			['                                                 ', 7]
		],
		[
			[' ', 23],
			['\u2502   \u2502 \u251c\u2574', 26],
			['\udb81\udee6 ', 24],
			['feedbackR', 0],
			['\u2502', 10],
			['   8   ', 11],
			['\u2502', 18],
			[' ', 7],
			['/** Prometheus /metrics + /healthz HTTP surface.', 29]
		],
		[
			[' NORMAL ', 42],
			['\ue0b0 \ue0a0 fetch-timeout ', 43],
			['\ue0b0', 26],
			[' \udb81\udee6 ', 24],
			[' apps/\u2026/src/', 44],
			['config.ts', 45],
			['    ', 44],
			['\ue0b2', 26],
			[' 31%  21:3  \ue0b2', 43],
			[' \uf43a 02:08 ', 42]
		]
	];

	// Cursor cell as tmux reported it (#{cursor_x}, #{cursor_y}).
	const CURSOR = { x: 30, y: 10 };

	// Split each run into plain ASCII text and single non-ASCII characters,
	// and mark the cursor's cell.
	// Box drawing is drawn in CSS to fill the cell, as terminals draw it, so
	// the explorer's frame and tree lines join whatever the font.
	const BOX: Record<string, string> = {
		'─': 'h',
		'│': 'v',
		'├': 'vr',
		'└': 'ur',
		'╴': 'l',
		'╭': 'dr',
		'╮': 'dl',
		'╰': 'ur r',
		'╯': 'ul',
		'▎': 'q',
		'▕': 'e'
	};
	type Piece = { t: string; s: number; cell: boolean; cursor: boolean; box?: string };
	const lines: Piece[][] = rows.map((row, y) => {
		const out: Piece[] = [];
		let x = 0;
		for (const [text, s] of row) {
			let buf = '';
			for (const ch of text) {
				const wide = ch.charCodeAt(0) > 0x7e;
				const cursor = y === CURSOR.y && x === CURSOR.x;
				if (wide || cursor) {
					if (buf) out.push({ t: buf, s, cell: false, cursor: false });
					buf = '';
					const box = BOX[ch];
					out.push({ t: box ? '' : ch, s, cell: true, cursor, box });
				} else buf += ch;
				x++;
			}
			if (buf) out.push({ t: buf, s, cell: false, cursor: false });
		}
		return out;
	});
</script>

<div class="min-w-0">
	<div class="h-60 overflow-hidden rounded-xs border border-[var(--rule)] bg-[var(--sunken)]">
		<div
			class="screen"
			role="img"
			aria-label="Neovim with LazyVim running on the machine, full screen: a narrow file explorer of the repo on the left, two of the worker's files open in tabs, and its TypeScript config in the editor."
		>
			{#each lines as line, y (y)}
				<div class="ln" aria-hidden="true">
					{#each line as p, i (i)}<span
							class:c={p.cell}
							class:cur={p.cursor}
							class={p.box ? `bx ${p.box}` : undefined}
							style={styles[p.s]}>{p.t}</span
						>{/each}
				</div>
			{/each}
		</div>
	</div>
	<h3 class="mt-5 text-lg font-semibold">Open it in your editor</h3>
	<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
		Each machine is an SSH host named after your repo, so Neovim runs right on it and VS Code,
		Cursor or Zed connect over SSH.
	</p>
</div>

<style>
	@font-face {
		font-family: 'Editor Nerd Symbols';
		src: url('/landing/editor-nerd-symbols.woff2') format('woff2');
		unicode-range: U+E0A0, U+E0B0, U+E0B2, U+EA76, U+F105, U+F43A, U+F024B, U+F06E6, U+F0770;
		font-display: block;
	}
	.screen {
		height: 100%;
		background: #222436;
		font-family:
			'Editor Nerd Symbols', 'JetBrains Mono', 'SF Mono', Menlo, 'DejaVu Sans Mono', Consolas,
			monospace;
		font-size: 10px;
		font-variant-ligatures: none;
		color: #c8d3f5;
	}
	.ln {
		height: 11.9px;
		line-height: 11.9px;
		white-space: pre;
		overflow: hidden;
	}
	/* One terminal cell: keeps the grid whatever font draws the glyph. */
	.c {
		display: inline-block;
		width: 1ch;
		height: 11.9px;
		vertical-align: top;
		overflow: visible;
		/* A Nerd Font icon is wider than a cell; like most terminals, let it
		   spill over the space nvim leaves after it instead of clipping it. */
		position: relative;
		z-index: 1;
	}
	.bx::before,
	.bx::after {
		content: '';
		position: absolute;
		border: 0 solid currentColor;
	}
	/* │ ├ └: the vertical stroke */
	.v::before,
	.vr::before {
		left: calc(50% - 0.5px);
		top: 0;
		bottom: 0;
		border-left-width: 1px;
	}
	/* ─ ╴ and the right arm of ├ */
	.h::before,
	.l::before,
	.vr::after {
		top: calc(50% - 0.5px);
		border-top-width: 1px;
	}
	.h::before {
		left: 0;
		right: 0;
	}
	.l::before {
		left: 0;
		right: 50%;
	}
	.vr::after {
		left: 50%;
		right: 0;
	}
	/* rounded corners ╭ ╮ ╰ ╯ and └ */
	.dr::before,
	.dl::before,
	.ur::before,
	.ul::before {
		width: calc(50% + 0.5px);
		height: calc(50% + 0.5px);
	}
	.dr::before {
		left: calc(50% - 0.5px);
		top: calc(50% - 0.5px);
		border-top-width: 1px;
		border-left-width: 1px;
		border-top-left-radius: 3px;
	}
	.dl::before {
		right: calc(50% - 0.5px);
		top: calc(50% - 0.5px);
		border-top-width: 1px;
		border-right-width: 1px;
		border-top-right-radius: 3px;
	}
	.ur::before {
		left: calc(50% - 0.5px);
		top: 0;
		border-bottom-width: 1px;
		border-left-width: 1px;
	}
	.ur.r::before {
		border-bottom-left-radius: 3px;
	}
	.ul::before {
		right: calc(50% - 0.5px);
		top: 0;
		border-bottom-width: 1px;
		border-right-width: 1px;
		border-bottom-right-radius: 3px;
	}
	/* ▎ left quarter block, ▕ right eighth block */
	.q::before {
		left: 0;
		top: 0;
		bottom: 0;
		width: 25%;
		background: currentColor;
	}
	.e::before {
		right: 0;
		top: 0;
		bottom: 0;
		width: 12.5%;
		background: currentColor;
	}
	.cur {
		background: #c8d3f5 !important;
		color: #222436 !important;
	}
</style>
