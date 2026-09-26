<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listDestroyed, listProjects } from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { pollWhileVisible } from '$lib/poll';
	import { money, normalizeRemoteDisplay, uptime } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import StateDot from '$lib/components/StateDot.svelte';
	import { abuseStopReason } from '$lib/abuse';
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

	/**
	 * The sentence after "code: " in last_error, for a project in error or
	 * one the platform stopped because a miner was running (I-239).
	 */
	function reason(p: Project): string {
		const abuse = abuseStopReason(p);
		if (abuse) return abuse;
		if (p.state !== 'error' || !p.last_error) return '';
		const i = p.last_error.indexOf(': ');
		return i > 0 ? p.last_error.slice(i + 2) : p.last_error;
	}

	onMount(() => pollWhileVisible(refresh));

	function agentSummary(p: Project): string {
		const agents = p.signals?.agents ?? [];
		if (agents.length === 0) return '—';
		return agents.map((a) => `${a.agent} · ${a.state}`).join(', ');
	}

	let summary = $derived.by(() => {
		if (!projects || projects.length === 0) return undefined;
		const awake = projects.filter((p) => p.state === 'running').length;
		const month = projects.reduce((n, p) => n + (p.cost_month_cents ?? 0), 0);
		const parts = [
			`${projects.length} ${projects.length === 1 ? 'project' : 'projects'}`,
			`${awake} running`,
			`${money(month)} this month`
		];
		return parts.join(' · ');
	});
</script>

<svelte:head>
	<title>Projects — repose</title>
</svelte:head>

<PageShell title="Projects" lede={summary}>
	{#if projects === undefined}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else if projects.length === 0}
		<div class="max-w-xl">
			<h2 class="text-xl font-semibold">No projects yet</h2>
			<p class="mt-2 text-zinc-600 dark:text-zinc-400">
				Projects are created from the CLI, in a git checkout. Install it, then run
				<code>repose run</code> in the project’s directory.
			</p>
			<pre class="codeblock mt-5">curl -fsSL https://repose.herakraft.co/install.sh | sh
repose login
cd ~/code/your-project && repose run</pre>
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="table">
				<thead>
					<tr>
						<th>Project</th>
						<th>State</th>
						<th>Size</th>
						<th>Agents</th>
						<th class="text-right">Today</th>
						<th class="text-right">This month</th>
					</tr>
				</thead>
				<tbody>
					{#each projects as p (p.id)}
						<tr class="group">
							<td>
								<a
									href={resolve('/projects/[id]', { id: p.id })}
									class="font-medium underline-offset-4 group-hover:underline">{p.name}</a
								>
								{#if p.remote_url}
									<div class="mt-0.5 font-mono text-xs text-zinc-500 dark:text-zinc-400">
										{normalizeRemoteDisplay(p.remote_url)}
									</div>
								{/if}
							</td>
							<td>
								<StateDot state={p.state} />
								{#if p.state === 'running'}
									<div class="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
										up {uptime(p.started_at)}
									</div>
								{/if}
								{#if p.state === 'running' && p.idle}
									<!-- Nobody on it for a day, still billing (I-262). -->
									<div class="mt-0.5 text-xs text-amber-700 dark:text-amber-400">
										idle {uptime(p.idle.since)} · ~{money(p.idle.hourly_cents)}/h
									</div>
								{/if}
								{#if reason(p)}
									<div class="mt-0.5 max-w-xs text-xs text-red-700 dark:text-red-400">
										{reason(p)}
									</div>
								{/if}
							</td>
							<td class="font-mono text-[13px]">{p.class}</td>
							<td class="text-zinc-600 dark:text-zinc-400">{agentSummary(p)}</td>
							<td class="text-right font-mono text-[13px]">{money(p.cost_today_cents)}</td>
							<td class="text-right font-mono text-[13px]">{money(p.cost_month_cents)}</td>
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
