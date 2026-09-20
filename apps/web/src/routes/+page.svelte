<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { signIn } from '$lib/auth.svelte';

	const INSTALL_COMMAND = 'curl -fsSL https://repose.herakraft.co/install.sh | sh';

	let signingIn = $state(false);

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

	const tiers = [
		{ name: 'small', vcpu: 2, ram: '4 GB', volume: '20 GB', cap: 49 },
		{ name: 'large', vcpu: 4, ram: '8 GB', volume: '40 GB', cap: 99 },
		{ name: 'xl', vcpu: 8, ram: '16 GB', volume: '80 GB', cap: 199 }
	];
</script>

<svelte:head>
	<title>repose — a persistent environment for coding agents</title>
</svelte:head>

<div class="mx-auto max-w-3xl px-5 pt-16 pb-24">
	<h1 class="font-display text-4xl font-semibold text-balance sm:text-5xl">
		A developer's laptop is the wrong place for a coding agent to run for six hours.
	</h1>
	<p class="mt-5 max-w-xl text-lg text-zinc-600 dark:text-zinc-400">
		repose gives each of your projects a persistent Linux environment on a shared server,
		reachable over SSH, where your agent keeps working after your laptop closes.
	</p>

	<div class="mt-8 flex flex-wrap items-center gap-4">
		<button type="button" class="btn" disabled={signingIn} onclick={onSignIn}>
			Sign in with GitHub
		</button>
		<code class="codeblock px-3 py-2 text-sm">{INSTALL_COMMAND}</code>
	</div>

	<div class="form-section">
		<h2 class="font-display text-xl font-semibold">What it looks like</h2>
		<pre class="codeblock mt-4">$ cd ~/code/todo-app
$ repose login                     # browser opens, Logto, GitHub sign-in, done
$ repose run                       # creates project "todo-app" on first use,
                                    # boots a guest, syncs the working tree,
                                    # attaches to tmux inside it
dev@todo-app:~/todo-app$            # tmux session "todo-app", window "shell"</pre>
	</div>

	<div class="form-section">
		<h2 class="font-display text-xl font-semibold">Pricing</h2>
		<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
			Billed hourly with a monthly cap per project, from the first hour. New accounts get $10 of
			trial credit; a card is required before the first environment starts.
		</p>
		<div class="mt-4 overflow-x-auto">
			<table class="w-full border-collapse text-sm">
				<thead>
					<tr class="border-b border-zinc-200 dark:border-zinc-800">
						<th class="px-2 py-2 text-left font-medium text-zinc-500 dark:text-zinc-400">Class</th>
						<th class="px-2 py-2 text-left font-medium text-zinc-500 dark:text-zinc-400">vCPU</th>
						<th class="px-2 py-2 text-left font-medium text-zinc-500 dark:text-zinc-400">RAM</th>
						<th class="px-2 py-2 text-left font-medium text-zinc-500 dark:text-zinc-400"
							>Default volume</th
						>
						<th class="px-2 py-2 text-left font-medium text-zinc-500 dark:text-zinc-400"
							>Monthly cap</th
						>
					</tr>
				</thead>
				<tbody>
					{#each tiers as tier (tier.name)}
						<tr class="border-t border-zinc-100 dark:border-zinc-900">
							<td class="px-2 py-2 font-medium">{tier.name}</td>
							<td class="px-2 py-2">{tier.vcpu}</td>
							<td class="px-2 py-2">{tier.ram}</td>
							<td class="px-2 py-2">{tier.volume}</td>
							<td class="px-2 py-2">${tier.cap}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
			Plus $0.10/GB-month of allocated storage and $0.05/GB of egress beyond 500 GB included per
			project. A guest that never stops pays the cap and never more.
		</p>
	</div>

	<p class="form-section text-sm text-zinc-500 dark:text-zinc-400">
		<a href={resolve('/terms')} class="link">Terms</a>
		<span class="mx-1.5">·</span>
		<a href={resolve('/privacy')} class="link">Privacy</a>
	</p>
</div>
