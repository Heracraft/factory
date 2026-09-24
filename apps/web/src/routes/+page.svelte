<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Session from '$lib/components/illustrations/Session.svelte';

	const INSTALL_COMMAND = 'curl -fsSL https://repose.herakraft.co/install.sh | sh';
	const SOURCE_URL = 'https://github.com/Heracraft/factory';

	let signingIn = $state(false);
	let copied = $state(false);

	onMount(() => {
		// Logto sends the user back here (not /callback) when sign-in itself
		// was cancelled or failed before a code was issued.
		const params = new URLSearchParams(location.search);
		if (params.get('error')) {
			toast.error('Sign-in was cancelled or failed; try again.');
			history.replaceState(null, '', location.pathname);
		}
	});

	async function onSignIn() {
		signingIn = true;
		try {
			await signIn();
		} catch {
			signingIn = false;
			toast.error('Sign-in was cancelled or failed; try again.');
		}
	}

	async function copyInstall() {
		try {
			await navigator.clipboard.writeText(INSTALL_COMMAND);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			toast.error('Could not copy. Select the command instead.');
		}
	}

	// Copy below marks commands with backticks; `inline` splits on them so
	// the odd-numbered parts render as code.
	function inline(text: string): { text: string; code: boolean }[] {
		return text.split('`').map((part, i) => ({ text: part, code: i % 2 === 1 }));
	}

	const facts = [
		{
			title: 'Your working state',
			text: '`repose run` in any checkout brings your uncommitted edits, unpushed commits, flake.nix, secrets and git logins. The toolchain is already installed.'
		},
		{
			title: 'Check in from anywhere',
			text: 'Close the laptop. You get a notification when the agent needs you, and `repose attach` picks up the same session from any computer.'
		},
		{
			title: 'One machine per project',
			text: 'Each project is isolated. Destroy one in a second; `repose restore` brings it back for 30 days.'
		}
	];

	// Each feature's sample is what the CLI or a phone actually shows, taken
	// from docs/features/. Lines starting with "$ " are commands.
	const features = [
		{
			title: 'A browser the agent can drive',
			text: 'Headless Chromium with Playwright MCP and chrome-devtools-mcp, registered in Claude Code already. `repose open --desktop` puts that browser in a tab on your laptop, so you can watch the agent or take over.',
			sample: ['$ repose open --desktop', 'http://localhost:6080/vnc.html?autoconnect=1']
		},
		{
			title: 'Five agents, one notification feed',
			text: 'Claude Code, Codex, opencode, Gemini CLI and pi come installed and wired to notifications. You hear by email or ntfy when one finishes or asks a question.',
			sample: [
				'$ repose notify set --ntfy https://ntfy.sh/your-topic',
				'',
				'repose · todo-app',
				'codex needs input: "Should I drop the legacy sessions table?"'
			]
		},
		{
			title: 'Packages from a menu, or from Nix',
			text: 'Add any nixpkgs package by name. The host builds it and the running machine switches over, no reboot for packages. Nix users edit the fragment itself. A build that fails leaves the previous revision running.',
			sample: [
				'$ repose config add bun gcc air',
				'Added bun, gcc and air to todo-app. Building revision 4f1c2a9e ...',
				'Applied revision 4f1c2a9e.'
			]
		},
		{
			title: 'Secrets that stay put',
			text: 'Named secrets are stored encrypted and appear in the machine as environment variables and files on a tmpfs. Your gh, Codex, opencode, Vercel and git logins copy over from the laptop inside SSH. You log in to Claude Code inside the machine; its credentials are never copied.',
			sample: [
				'$ repose secrets set DATABASE_URL',
				'Stored for todo-app. Available as $DATABASE_URL and',
				'/run/repose/secrets/DATABASE_URL.'
			]
		},
		{
			title: 'Snapshots every night and every stop',
			text: 'Each project keeps seven days of snapshots. Restore one in place, or next to the original as a new project.',
			sample: [
				'$ repose snapshots restore snap_01J8… --as-new todo-app-yesterday',
				'Created todo-app-yesterday (large) from snap_01J8… ... done.'
			]
		},
		{
			title: 'Docker and databases next to the code',
			text: 'Each machine is a NixOS microVM with its own kernel and disk, and Docker runs out of the box. Postgres, Redis, MySQL, RabbitMQ, NATS and Meilisearch are one command away.',
			sample: [
				'$ docker compose up -d',
				'$ repose config add postgresql redis',
				'Applied revision 9a8e77d0.'
			]
		}
	];

	// The ports block's terminal, row by row, as [text, colour class] runs:
	// two servers in the machine, which the status bar below forwards.
	const PROMPT = 'text-[#67c1d4]';
	const ARROW = 'text-[#c678dd]';
	const portsDemo: [string, string][][] = [
		[
			['~/todo-app ', PROMPT],
			['❯ ', ARROW],
			['pnpm dev', '']
		],
		[],
		[
			['  VITE ', 'text-[#98c379]'],
			['v8.1.5  ready in 743 ms', '']
		],
		[
			['  ➜  Local:   ', ''],
			['http://localhost:5173/', PROMPT]
		],
		[],
		[
			['~/todo-app/api ', PROMPT],
			['❯ ', ARROW],
			['go run .', '']
		],
		[['listening on :3000', '']]
	];

	const steps = [
		{
			title: 'Install the CLI',
			text: 'One binary for macOS and Linux.',
			command: INSTALL_COMMAND
		},
		{
			title: 'Sign in',
			text: 'GitHub sign-in opens in your browser. Add a card once in the dashboard.',
			command: 'repose login'
		},
		{
			title: 'Run in any checkout',
			text: 'The first run creates the machine, syncs your work and attaches you to it.',
			command: 'cd ~/code/izma && repose run'
		}
	];
	const runOutput: [string, string][] = [
		['Created izma (large)', '0.1s'],
		['Built the environment', '5.3s'],
		['Booted izma', '11s'],
		['Synced: 4 modified, 2 untracked (3 new commits)', ''],
		['Ready in 15s.', '']
	];

	const tiers = [
		{ name: 'small', vcpu: 2, ram: '4 GB', disk: '20 GB', hour: '$0.07', cap: '$49' },
		{ name: 'large', vcpu: 4, ram: '8 GB', disk: '40 GB', hour: '$0.14', cap: '$99' },
		{ name: 'xl', vcpu: 8, ram: '16 GB', disk: '80 GB', hour: '$0.28', cap: '$199' }
	];

	const pricingNotes = [
		'A machine left running all month costs exactly its monthly cap. Stop it at night and you pay less.',
		'A stopped machine is charged for its disk only. `repose destroy` ends that too, and keeps a snapshot for 30 days.',
		'Your first day of compute is on us: a day on large, or two on small.',
		'`repose status` shows what each project has cost so far today.'
	];
</script>

<svelte:head>
	<title>repose: let your agents run with full permissions</title>
	<meta
		name="description"
		content="A disposable dev machine per project with your code, tools and secrets on it in 15 seconds, so coding agents can run with full permissions and your laptop stays out of reach."
	/>
</svelte:head>

{#snippet rich(text: string)}{#each inline(text) as part, i (i)}{#if part.code}<code
				class="rounded-xs bg-[var(--sunken)] px-1 py-px text-[0.88em] whitespace-nowrap text-zinc-800 dark:text-zinc-200"
				>{part.text}</code
			>{:else}{part.text}{/if}{/each}{/snippet}

{#snippet sample(lines: string[])}
	<pre class="codeblock !text-[12.5px] !leading-6">{#each lines as line, i (i)}<span
				class="block min-h-6"
				>{#if line.startsWith('$ ')}<span class="text-zinc-600 select-none dark:text-zinc-400"
						>$ </span>{line.slice(2)}{:else}<span class="text-zinc-600 dark:text-zinc-400"
						>{line}</span
					>{/if}</span
			>{/each}</pre>
{/snippet}

<header class="border-b border-[var(--rule)]">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between gap-4 px-5">
		<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
		<nav class="flex items-center gap-6 text-sm" aria-label="Main">
			<a
				href={resolve('/docs')}
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Docs</a
			>
			<a
				href="#features"
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Features</a
			>
			<a
				href="#pricing"
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Pricing</a
			>
			<a
				href={SOURCE_URL}
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>GitHub</a
			>
			<button type="button" class="btn-quiet !py-1.5" disabled={signingIn} onclick={onSignIn}
				>Sign in</button
			>
		</nav>
	</div>
</header>

<main>
	<section class="mx-auto max-w-5xl px-5 pt-12 pb-16">
		<h1 class="max-w-3xl text-4xl leading-tight font-semibold sm:text-5xl sm:leading-[1.12]">
			Let your agents run with full permissions
		</h1>
		<p class="mt-5 max-w-2xl text-lg leading-relaxed text-zinc-600 dark:text-zinc-400">
			Your code, tools and secrets on a disposable machine in 15 seconds. Leave them running for as
			long as the work takes.
		</p>
		<p class="mt-3 max-w-2xl leading-relaxed text-zinc-600 dark:text-zinc-400">
			An agent in bypass mode reaches one project's microVM and nothing else. Your laptop, your SSH
			keys and your other projects stay out of its reach.
		</p>

		<div class="mt-7 flex flex-wrap items-center gap-3">
			<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
				Sign in with GitHub
			</button>
			<div
				class="flex max-w-full min-w-0 items-center gap-2 rounded-sm border border-[var(--rule-strong)] bg-[var(--sunken)] py-1.5 pr-1.5 pl-3.5 font-mono text-[13px]"
			>
				<span class="truncate select-all">{INSTALL_COMMAND}</span>
				<button
					type="button"
					onclick={copyInstall}
					class="inline-flex shrink-0 cursor-pointer items-center gap-1.5 rounded-sm border border-[var(--rule-strong)] bg-[var(--surface)] px-2 py-1 font-sans text-xs text-zinc-700 hover:border-zinc-500 dark:text-zinc-300"
					aria-label="Copy the install command"
				>
					<svg
						viewBox="0 0 16 16"
						class="h-3.5 w-3.5"
						fill="none"
						stroke="currentColor"
						stroke-width="1.4"
						aria-hidden="true"
						><rect x="5.5" y="5.5" width="8" height="8" rx="1" /><path
							d="M10.5 3.5v-1h-8v8h1"
						/></svg
					>
					{copied ? 'Copied' : 'Copy'}
				</button>
			</div>
		</div>

		<figure class="mt-8">
			<Session />
			<figcaption class="mt-3 text-sm text-zinc-600 dark:text-zinc-400">
				Recorded on a repose machine: Claude Code fixes a bug while the app's dev server reloads.
				The notification and the reattach at the end are illustrated.
			</figcaption>
		</figure>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-10 px-5 py-16 sm:grid-cols-3">
			{#each facts as f (f.title)}
				<div>
					<h2 class="text-lg font-semibold">{f.title}</h2>
					<p class="mt-2 text-zinc-600 dark:text-zinc-400">{@render rich(f.text)}</p>
				</div>
			{/each}
		</div>
	</section>

	<section id="features" class="scroll-mt-6 border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<h2 class="text-2xl font-semibold">On every machine</h2>
			<p class="mt-2 max-w-2xl text-zinc-600 dark:text-zinc-400">
				All of this ships in the base image, so a new project has it from the first boot.
			</p>

			<div
				class="mt-10 grid gap-8 border-y border-[var(--rule)] py-8 md:grid-cols-[minmax(0,5fr)_minmax(0,6fr)] md:items-center"
			>
				<div>
					<h3 class="text-xl font-semibold">Your dev server, on your localhost</h3>
					<p class="mt-2 text-zinc-600 dark:text-zinc-400">
						While you're attached, every port the machine listens on appears on your laptop's
						localhost within a second. Start {@render rich('`pnpm dev`')} in the machine and open
						{@render rich('`http://localhost:5173`')}: the same cookies, OAuth redirects and secure
						context as local work, carried over SSH. Nothing is exposed to the internet.
					</p>
					<p class="mt-3 text-zinc-600 dark:text-zinc-400">
						A port your laptop already uses moves to the next free one, and tmux tells you where.
						{@render rich('`repose open 3000`')} forwards one port on demand.
					</p>
				</div>
				<div class="min-w-0">
					<div
						class="overflow-x-auto rounded-sm border border-[#2a2a28] bg-[#0d0d0c] font-mono text-[12.5px] leading-6 text-[#d4d4d0]"
					>
						<div class="px-4 pt-3 pb-4">
							{#each portsDemo as row, i (i)}
								<div class="h-6 whitespace-pre">
									{#each row as [text, tone], j (j)}<span class={tone}>{text}</span>{/each}
								</div>
							{/each}
						</div>
						<div class="flex justify-between gap-4 bg-[#1f9d55] px-4 whitespace-pre text-[#0b0b0b]">
							<span>[todo-app] 0:dev*</span><span>⇄ 3000 5173 │ 14:02</span>
						</div>
					</div>
					<pre class="codeblock mt-3 !text-[12.5px] !leading-6">⇄ localhost:5173 → :5173
⇄ localhost:3001 → :3000 <span class="text-zinc-600 dark:text-zinc-400"
							>(3000 is taken on your laptop)</span
						></pre>
				</div>
			</div>

			<div class="mt-10 grid gap-x-10 gap-y-12 md:grid-cols-2">
				{#each features as f (f.title)}
					<div class="min-w-0">
						<h3 class="text-lg font-semibold">{f.title}</h3>
						<p class="mt-2 text-zinc-600 dark:text-zinc-400">{@render rich(f.text)}</p>
						<div class="mt-4">{@render sample(f.sample)}</div>
					</div>
				{/each}
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<h2 class="text-2xl font-semibold">Start in three commands</h2>
			<ol class="mt-8 border-b border-[var(--rule)]">
				{#each steps as step, i (step.title)}
					<li
						class="grid gap-3 border-t border-[var(--rule)] py-6 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:gap-10"
					>
						<div>
							<h3 class="font-semibold">
								<span class="mr-2 font-mono text-sm text-zinc-600 dark:text-zinc-400">{i + 1}</span
								>{step.title}
							</h3>
							<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">{step.text}</p>
						</div>
						<div class="min-w-0">
							{@render sample([`$ ${step.command}`])}
							{#if i === steps.length - 1}
								<div class="mt-2 overflow-x-auto">
									<!-- prettier-ignore -->
									<pre class="codeblock !text-[12.5px] !leading-6 !whitespace-pre">{#each runOutput as [text, time] (text)}<span class="block">{#if time}<span class="text-emerald-600 dark:text-emerald-400">✓</span> {text.padEnd(26)}<span class="text-zinc-600 dark:text-zinc-400">{time}</span>{:else}{text}{/if}</span>{/each}</pre>
								</div>
							{/if}
						</div>
					</li>
				{/each}
			</ol>
		</div>
	</section>

	<section id="pricing" class="scroll-mt-6 border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<h2 class="text-2xl font-semibold">Pricing</h2>
			<p class="mt-2 text-zinc-600 dark:text-zinc-400">
				Per hour while a machine runs, capped each month.
			</p>

			<div class="mt-6 hidden sm:block">
				<table class="table">
					<thead>
						<tr>
							<th>Size</th>
							<th>vCPU</th>
							<th>Memory</th>
							<th>Disk</th>
							<th class="text-right">Per hour</th>
							<th class="text-right">Monthly cap</th>
						</tr>
					</thead>
					<tbody>
						{#each tiers as t (t.name)}
							<tr>
								<td class="font-mono">{t.name}</td>
								<td>{t.vcpu}</td>
								<td>{t.ram}</td>
								<td>{t.disk}</td>
								<td class="text-right font-mono">{t.hour}</td>
								<td class="text-right font-mono">{t.cap}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<ul class="mt-6 sm:hidden">
				{#each tiers as t (t.name)}
					<li class="row flex items-baseline justify-between gap-4">
						<div>
							<p class="font-mono">{t.name}</p>
							<p class="mt-0.5 text-sm text-zinc-600 dark:text-zinc-400">
								{t.vcpu} vCPU · {t.ram} · {t.disk} disk
							</p>
						</div>
						<div class="text-right">
							<p class="font-mono">{t.hour}<span class="text-sm text-zinc-600"> /h</span></p>
							<p class="mt-0.5 text-sm text-zinc-600 dark:text-zinc-400">cap {t.cap}/mo</p>
						</div>
					</li>
				{/each}
			</ul>

			<ul
				class="mt-6 grid gap-x-10 gap-y-3 text-sm text-zinc-600 sm:grid-cols-2 dark:text-zinc-400"
			>
				{#each pricingNotes as note (note)}
					<li class="border-l-2 border-[var(--rule-strong)] pl-3">{@render rich(note)}</li>
				{/each}
				<li class="border-l-2 border-[var(--rule-strong)] pl-3">
					Disk $0.10 per GB-month. 500 GB egress included per project, then $0.05 per GB.
				</li>
			</ul>

			<div class="mt-10 flex flex-wrap items-center gap-4">
				<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
					Start with GitHub
				</button>
				<p class="text-sm text-zinc-600 dark:text-zinc-400">
					A card is needed before the first machine starts.
				</p>
			</div>
		</div>
	</section>
</main>

<footer class="border-t border-[var(--rule)]">
	<div
		class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-5 py-8 text-sm text-zinc-600 dark:text-zinc-400"
	>
		<Logo size="sm" />
		<nav class="flex gap-6" aria-label="Footer">
			<a href={resolve('/docs')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Docs</a>
			<a href={SOURCE_URL} class="hover:text-zinc-900 dark:hover:text-zinc-100">GitHub</a>
			<a href={resolve('/terms')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Terms</a>
			<a href={resolve('/privacy')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Privacy</a>
		</nav>
	</div>
</footer>
