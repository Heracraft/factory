<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { listDestroyed, listProjects } from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { pollWhileVisible } from '$lib/poll';
	import { money, uptime } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import StateDot from '$lib/components/StateDot.svelte';
	import RecentlyDestroyed from '$lib/components/RecentlyDestroyed.svelte';
	import type { DestroyedProject, Project } from '$lib/api/types';

	let projects = $state<Project[] | undefined>(undefined);
	let destroyed = $state<DestroyedProject[]>([]);

	async function refresh() {
		try {
			projects = await listProjects();
		} catch (err) {
			toastApiError(err, 'Could not load projects.');
			// Not asked while the list fails: its answer would reset the
			// "cannot reach the api" bar the failed list just raised.
			return;
		}
		// Its own try: an api without the route (older than I-167) answers
		// 404, and that must not hide the live list or raise a toast.
		try {
			destroyed = await listDestroyed();
		} catch {
			destroyed = [];
		}
	}

	/** The sentence after "code: " in last_error, for a project in error. */
	function reason(p: Project): string {
		if (p.state !== 'error' || !p.last_error) return '';
		const i = p.last_error.indexOf(': ');
		return i > 0 ? p.last_error.slice(i + 2) : p.last_error;
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
							onclick={() => goto(resolve('/projects/[id]', { id: p.id }))}
						>
							<td class="px-2 py-2.5 font-medium">
								<a href={resolve('/projects/[id]', { id: p.id })} class="hover:underline"
									>{p.name}</a
								>
							</td>
							<td class="px-2 py-2.5">{p.class}</td>
							<td class="px-2 py-2.5">
								<StateDot state={p.state} />
								{#if reason(p)}
									<p class="mt-0.5 max-w-xs text-xs text-red-600 dark:text-red-400">{reason(p)}</p>
								{/if}
							</td>
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
	{#if projects !== undefined}
		<RecentlyDestroyed
			{destroyed}
			liveSlugs={(projects ?? []).map((p) => p.slug)}
			onrestored={() => void refresh()}
		/>
	{/if}
</PageShell>
