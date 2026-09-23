<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Session from '$lib/components/illustrations/Session.svelte';

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

	const points = [
		{
			title: 'Ready on first boot',
			text: 'Node, Python, Go, Rust and Docker are installed. Your repo’s `flake.nix` works as it does locally, and your secrets and git logins come with you.'
		},
		{
			title: 'Yours from any computer',
			text: '`repose attach izma` from any laptop with the CLI, or plain `ssh izma.repose` from your editor.'
		},
		{
			title: 'Full permissions, no risk',
			text: 'Run Claude Code with `--dangerously-skip-permissions` for hours. It can only touch this machine, never your laptop, and a snapshot rolls it back.'
		}
	];

	const tiers = [
		{ name: 'small', vcpu: 2, ram: '4 GB', disk: '20 GB', hour: '$0.07', cap: '$49' },
		{ name: 'large', vcpu: 4, ram: '8 GB', disk: '40 GB', hour: '$0.14', cap: '$99' },
		{ name: 'xl', vcpu: 8, ram: '16 GB', disk: '80 GB', hour: '$0.28', cap: '$199' }
	];
</script>

<svelte:head>
	<title>repose: a dev machine for every project, set up and always on</title>
	<meta
		name="description"
		content="A dev machine for every project: set up on first boot, reachable from any computer, and safe to give coding agents full permissions."
	/>
</svelte:head>

<header class="border-b border-[var(--rule)]">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between px-5">
		<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
		<nav class="flex items-center gap-6 text-sm">
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
	<section class="mx-auto max-w-5xl px-5 pt-14 pb-16">
		<h1 class="max-w-3xl text-4xl leading-tight font-semibold sm:text-5xl sm:leading-[1.12]">
			A dev machine for every project, set up and always on
		</h1>
		<dl class="mt-8 grid max-w-4xl gap-x-10 gap-y-5 sm:grid-cols-3">
			{#each points as pt (pt.title)}
				<div class="border-t border-[var(--rule-strong)] pt-3">
					<dt class="font-display font-semibold">{pt.title}</dt>
					<dd class="mt-1.5 text-[0.95rem] leading-relaxed text-zinc-600 dark:text-zinc-400">
						{#each pt.text.split('`') as part, i (i)}{#if i % 2}<code
									class="text-[0.9em] break-all text-zinc-900 sm:break-normal sm:whitespace-nowrap dark:text-zinc-100"
									>{part}</code
								>{:else}{part}{/if}{/each}
					</dd>
				</div>
			{/each}
		</dl>

		<div class="mt-8 flex flex-wrap items-center gap-3">
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

		<div class="mt-8">
			<Session />
		</div>
	</section>

	<section class="border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<h2 class="text-2xl font-semibold">Start</h2>
			<pre class="codeblock mt-6 !p-5 !text-[13px] !leading-7 !whitespace-pre"><span
					class="text-zinc-400">$</span
				> curl -fsSL https://repose.herakraft.co/install.sh | sh
<span class="text-zinc-400">$</span> repose login
<span class="text-zinc-400">$</span> cd ~/code/izma && repose run
<span class="text-emerald-600 dark:text-emerald-400">✓</span> Created izma (large)            <span
					class="text-zinc-400">0.1s</span
				>
<span class="text-emerald-600 dark:text-emerald-400">✓</span> Built the environment           <span
					class="text-zinc-400">5.3s</span
				>
<span class="text-emerald-600 dark:text-emerald-400">✓</span> Booted izma                      <span
					class="text-zinc-400">11s</span
				>
Synced: 4 modified, 2 untracked (3 new commits)
Ready in 15s.</pre>
		</div>
	</section>

	<section id="pricing" class="scroll-mt-6 border-t border-[var(--rule)]">
		<div class="mx-auto max-w-5xl px-5 py-16">
			<h2 class="text-2xl font-semibold">Pricing</h2>
			<p class="mt-2 text-zinc-600 dark:text-zinc-400">
				Per hour while a machine runs, capped each month. New accounts get $10 of credit.
			</p>
			<div class="mt-6 overflow-x-auto">
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
			<p class="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
				Disk $0.10 per GB-month. 500 GB egress included per project, then $0.05 per GB.
			</p>
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
