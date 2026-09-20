<script lang="ts">
	import { onMount } from 'svelte';
	import { listProjects } from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { pollWhileVisible } from '$lib/poll';
	import { money, uptime } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import StateDot from '$lib/components/StateDot.svelte';
	import type { Project } from '$lib/api/types';

	let projects = $state<Project[] | undefined>(undefined);

	async function refresh() {
		try {
			projects = await listProjects();
		} catch (err) {
			toastApiError(err, 'Could not load projects.');
		}
	}

	onMount(() => pollWhileVisible(refresh));

	function agentSummary(p: Project): string {
		const agents = p.signals?.agents ?? [];
		if (agents.length === 0) return '—';
		return agents.map((a) => `${a.agent}: ${a.state}`).join(', ');
	}
</script>

<svelte:head>
	<title>Projects — repose</title>
</svelte:head>

<PageShell title="Projects">
	{#if projects === undefined}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else if projects.length === 0}
		<div class="card">
			<p class="text-sm text-zinc-700 dark:text-zinc-300">
				No projects yet. Projects are created from the CLI, not from here — a project needs a git
				remote and a laptop-side sync.
			</p>
			<code class="codeblock mt-4 block px-3 py-2 text-sm"
				>curl -fsSL https://repose.herakraft.co/install.sh | sh</code
			>
			<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
				Then, in a project directory: <code class="font-mono">repose login && repose run</code>.
			</p>
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full border-collapse text-sm">
				<thead>
					<tr class="border-b border-zinc-200 text-left text-xs text-zinc-500 dark:border-zinc-800 dark:text-zinc-400">
						<th class="px-2 py-2 font-medium">Name</th>
						<th class="px-2 py-2 font-medium">Class</th>
						<th class="px-2 py-2 font-medium">State</th>
						<th class="px-2 py-2 font-medium">Uptime</th>
						<th class="px-2 py-2 font-medium">Agent</th>
						<th class="px-2 py-2 font-medium">Cost today</th>
						<th class="px-2 py-2 font-medium">Cost month</th>
					</tr>
				</thead>
				<tbody>
					{#each projects as p (p.id)}
						<tr
							class="cursor-pointer border-t border-zinc-100 hover:bg-zinc-100/60 dark:border-zinc-900 dark:hover:bg-zinc-900/60"
							onclick={() => (location.href = `/projects/${p.id}`)}
						>
							<td class="px-2 py-2.5 font-medium">
								<a href={`/projects/${p.id}`} class="hover:underline">{p.name}</a>
							</td>
							<td class="px-2 py-2.5">{p.class}</td>
							<td class="px-2 py-2.5"><StateDot state={p.state} /></td>
							<td class="px-2 py-2.5">{p.state === 'running' ? uptime(p.started_at) : '—'}</td>
							<td class="px-2 py-2.5">{agentSummary(p)}</td>
							<td class="px-2 py-2.5">{money(p.cost_today_cents)}</td>
							<td class="px-2 py-2.5">{money(p.cost_month_cents)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</PageShell>
