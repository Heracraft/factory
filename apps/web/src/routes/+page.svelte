<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Detach from '$lib/components/illustrations/Detach.svelte';
	import Sync from '$lib/components/illustrations/Sync.svelte';
	import Snapshots from '$lib/components/illustrations/Snapshots.svelte';

	const INSTALL_COMMAND = 'curl -fsSL https://repose.herakraft.co/install.sh | sh';

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

	const includes: [string, string][] = [
		[
			'Machine',
			'A NixOS virtual machine with 2, 4 or 8 vCPUs and its own disk. You reach it with ssh izma.repose, so editors with a remote mode work too.'
		],
		[
			'Agents',
			'Claude Code, Codex CLI, opencode, Gemini CLI and pi are installed and kept up to date. You sign in to each one inside the machine; tokens from your laptop are not copied.'
		],
		[
			'Ports',
			'repose open 3000 forwards a port from the machine to your browser. repose open --desktop starts a full desktop.'
		],
		['Notifications', 'An email or an ntfy push when an agent stops and waits for input.'],
		[
			'Privacy',
			'Our logs record ids, states, sizes and durations. They never record prompts, terminal output, file paths inside your machine, environment variables or secret values.'
		]
	];

	const commands: [string, string][] = [
		['repose run', 'Create or start the project, sync, and attach'],
		['repose run "PROMPT"', 'The same, and start an agent with the prompt'],
		['repose attach izma', 'Attach to the project’s tmux session'],
		['repose open 3000', 'Forward a port to your browser'],
		['repose stop', 'Snapshot the project and stop the machine'],
		['repose destroy', 'Delete the machine; the snapshot is kept 30 days'],
		['repose restore izma', 'Bring back a destroyed project'],
		['repose status', 'State, running agents and cost so far today']
	];

	const tiers = [
		{ name: 'small', vcpu: 2, ram: '4 GB', disk: '20 GB', hour: '$0.07', cap: '$49' },
		{ name: 'large', vcpu: 4, ram: '8 GB', disk: '40 GB', hour: '$0.14', cap: '$99' },
		{ name: 'xl', vcpu: 8, ram: '16 GB', disk: '80 GB', hour: '$0.28', cap: '$199' }
	];
</script>

<svelte:head>
	<title>repose: a development machine you can reach from anywhere</title>
	<meta
		name="description"
		content="A Linux development machine per project, set up for your stack, reachable from any computer, and left running for coding agents."
	/>
</svelte:head>

<header class="border-b border-[var(--rule)]">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between px-5">
		<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
		<nav class="flex items-center gap-6 text-sm">
			<a
				href="#how"
				class="hidden text-zinc-500 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>How it works</a
			>
			<a
				href="#pricing"
				class="hidden text-zinc-500 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>Pricing</a
			>
			<button type="button" class="btn-quiet !py-1.5" disabled={signingIn} onclick={onSignIn}
				>Sign in</button
			>
		</nav>
	</div>
</header>

<main>
	<section class="mx-auto max-w-5xl px-5 pt-20 pb-16">
		<h1 class="max-w-3xl text-4xl leading-tight font-semibold sm:text-5xl sm:leading-[1.12]">
			A development machine you can reach from anywhere
		</h1>
		<p class="mt-6 max-w-2xl text-lg leading-relaxed text-zinc-600 dark:text-zinc-400">
			repose gives each of your projects a Linux machine that is already set up for development:
			common toolchains installed, your code synced from your laptop, and your secrets and tool
			logins in place. You connect from any computer with the CLI or plain SSH. The machine keeps
			running when you disconnect, so you can leave a coding agent working on it.
		</p>

		<div class="mt-10 flex flex-wrap items-center gap-3">
			<button type="button" class="btn !px-5 !py-2.5" disabled={signingIn} onclick={onSignIn}>
				Sign in with GitHub
			</button>
			<button
				type="button"
				onclick={copyInstall}
				class="flex max-w-full min-w-0 cursor-pointer items-center gap-3 rounded-sm border border-[var(--rule-strong)] bg-[var(--sunken)] px-3.5 py-2.5 text-left font-mono text-[13px] hover:border-zinc-500"
				aria-label="Copy the install command"
			>
				<span class="truncate">{INSTALL_COMMAND}</span>
				<span class="shrink-0 font-sans text-xs text-zinc-500">{copied ? 'Copied' : 'Copy'}</span>
			</button>
		</div>
		<p class="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
			New accounts get $10 of credit. A card is required before the first machine starts.
		</p>

		<figure class="mt-16 border-t border-[var(--rule)] pt-12">
			<div class="mx-auto max-w-3xl">
				<Detach />
			</div>
			<figcaption class="mt-6 text-sm text-zinc-500 dark:text-zinc-400">
				The agent runs in tmux on the project’s machine. Closing the laptop only ends the SSH
				connection.
			</figcaption>
		</figure>
	</section>

	<section id="how" class="scroll-mt-6 border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<h2 class="text-3xl font-semibold">How it works</h2>
			<ol class="space-y-8">
				<li class="grid grid-cols-[2rem_1fr]">
					<span class="font-display text-zinc-400">1</span>
					<div>
						<h3 class="font-semibold">Install the CLI and sign in</h3>
						<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
							One binary for Linux and macOS. <code>repose login</code> opens a browser to sign in with
							GitHub.
						</p>
					</div>
				</li>
				<li class="grid grid-cols-[2rem_1fr]">
					<span class="font-display text-zinc-400">2</span>
					<div>
						<h3 class="font-semibold">Run it in a checkout</h3>
						<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
							<code>repose run</code> creates the project’s machine on first use, copies your working
							tree into it and opens a tmux session there. A new machine is ready in about 15 seconds.
						</p>
					</div>
				</li>
				<li class="grid grid-cols-[2rem_1fr]">
					<span class="font-display text-zinc-400">3</span>
					<div>
						<h3 class="font-semibold">Work in it, or hand it to an agent</h3>
						<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
							Use the session like any shell, open the machine in your editor over SSH, or start an
							agent with a prompt: <code>repose run "fix the flaky login test"</code>.
						</p>
					</div>
				</li>
				<li class="grid grid-cols-[2rem_1fr]">
					<span class="font-display text-zinc-400">4</span>
					<div>
						<h3 class="font-semibold">Leave, and come back</h3>
						<p class="mt-1.5 text-zinc-600 dark:text-zinc-400">
							Detach with <kbd>Ctrl-b d</kbd> or close the laptop. Later, from any computer with the
							CLI: <code>repose attach izma</code>.
						</p>
					</div>
				</li>
			</ol>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<h2 class="text-3xl font-semibold">Configured before you log in</h2>
			<div class="space-y-4 leading-relaxed text-zinc-600 dark:text-zinc-400">
				<p>
					Every machine starts from our NixOS base: Node 24 with pnpm, Python 3.12 with uv, Go,
					Rust, Docker, git, gh, direnv, tmux and neovim. Most projects need nothing else.
				</p>
				<p>
					If your repository has a <code>flake.nix</code>, its dev shell works the same way it does
					on your laptop, with <code>nix develop</code> or direnv. Services such as Postgres or
					Redis go in the project’s <code>repose.nix</code>, or you can pick them from a menu in the
					dashboard.
				</p>
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<h2 class="text-3xl font-semibold">Secrets and logins</h2>
			<div class="space-y-4 leading-relaxed text-zinc-600 dark:text-zinc-400">
				<p>
					Secrets you add with <code>repose secrets set</code> are stored encrypted and appear as environment
					variables in every shell on the machine. They are never written to its disk.
				</p>
				<p>
					Your logins for gh, Codex and opencode, and your git name and email, are copied from your
					laptop on each <code>repose run</code>. Claude Code is the exception: you sign in to it
					once, inside the machine.
				</p>
			</div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-20">
			<div class="grid gap-12 md:grid-cols-[1fr_1.4fr]">
				<h2 class="text-3xl font-semibold">Your working tree comes with you</h2>
				<p class="leading-relaxed text-zinc-600 dark:text-zinc-400">
					Each <code>repose run</code> sends the state of your checkout to the machine: commits you
					have not pushed as a git bundle, uncommitted changes as a diff, and untracked files as
					they are. The machine never needs access to your git host. Dependency folders such as
					<code>node_modules</code> stay behind; install them on the machine, where they are built for
					its platform.
				</p>
			</div>
			<div class="mt-14"><Sync /></div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-20">
			<div class="grid gap-12 md:grid-cols-[1fr_1.4fr]">
				<h2 class="text-3xl font-semibold">Snapshots</h2>
				<p class="leading-relaxed text-zinc-600 dark:text-zinc-400">
					Stopping a project snapshots its disk, and so does destroying it. A destroyed project’s
					last snapshot is kept for 30 days, and <code>repose restore izma</code> brings the project back
					under its old name. A stopped project costs only its disk.
				</p>
			</div>
			<div class="mt-14"><Snapshots /></div>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<h2 class="text-3xl font-semibold">What a project includes</h2>
			<dl class="border-t border-[var(--rule-strong)]">
				{#each includes as [term, text] (term)}
					<div
						class="grid gap-1 border-b border-[var(--rule)] py-4 sm:grid-cols-[9rem_1fr] sm:gap-6"
					>
						<dt class="font-display font-semibold">{term}</dt>
						<dd class="text-zinc-600 dark:text-zinc-400">{text}</dd>
					</div>
				{/each}
			</dl>
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<h2 class="text-3xl font-semibold">Commands</h2>
			<div class="overflow-x-auto">
				<table class="table">
					<tbody>
						{#each commands as [cmd, what] (cmd)}
							<tr>
								<td class="font-mono text-[13px] whitespace-nowrap">{cmd}</td>
								<td class="text-zinc-600 dark:text-zinc-400">{what}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</section>

	<section id="pricing" class="scroll-mt-6 border-t border-[var(--rule)]">
		<div class="mx-auto grid max-w-5xl gap-12 px-5 py-20 md:grid-cols-[1fr_1.4fr]">
			<div>
				<h2 class="text-3xl font-semibold">Pricing</h2>
				<p class="mt-4 text-zinc-600 dark:text-zinc-400">
					Charged per hour while a machine runs, up to a monthly cap per project.
				</p>
			</div>
			<div>
				<div class="overflow-x-auto">
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
				<p class="mt-6 text-sm leading-relaxed text-zinc-600 dark:text-zinc-400">
					Disk is $0.10 per GB per month for the size you allocate. Each project includes 500 GB of
					egress a month, then $0.05 per GB. New accounts get $10 of credit, used before the card is
					charged.
				</p>
			</div>
		</div>
	</section>
</main>

<footer class="border-t border-[var(--rule)]">
	<div
		class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-5 py-8 text-sm text-zinc-500 dark:text-zinc-400"
	>
		<Logo size="sm" />
		<nav class="flex gap-6">
			<a href={resolve('/terms')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Terms</a>
			<a href={resolve('/privacy')} class="hover:text-zinc-900 dark:hover:text-zinc-100">Privacy</a>
		</nav>
	</div>
</footer>
