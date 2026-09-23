<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import {
		getProject,
		startProject,
		stopProject,
		destroyProject,
		resizeProject,
		getOp,
		listEvents,
		listSnapshots,
		createSnapshot,
		restoreSnapshot,
		getUsage,
		listRevisions
	} from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { pollWhileVisible, pollUntilDone } from '$lib/poll';
	import {
		money,
		uptime,
		gb,
		relativeTime,
		dateTime,
		projectedMonthCents,
		normalizeRemoteDisplay
	} from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import StateDot from '$lib/components/StateDot.svelte';
	import ConfirmType from '$lib/components/ConfirmType.svelte';
	import type { Project, ProjectEvent, Snapshot, Revision } from '$lib/api/types';

	const id = page.params.id as string;

	let project = $state<Project | undefined>(undefined);
	let notFound = $state(false);
	let lastUpdated = $state<Date | undefined>(undefined);
	let events = $state<ProjectEvent[]>([]);
	let snapshots = $state<Snapshot[]>([]);
	let revisions = $state<Revision[]>([]);
	let projectedMonth = $state<number | undefined>(undefined);

	// Start/stop/destroy.
	let opBusy = $state<'start' | 'stop' | 'destroy' | 'resize' | 'snapshot' | 'restore' | undefined>(
		undefined
	);
	let startBanner = $state<'payment_required' | 'capacity' | undefined>(undefined);
	let stopSnapshotFirst = $state(true);

	// Resize.
	let showResize = $state(false);
	const SIZES_GB = [20, 40, 80, 160, 320];
	let resizeTo = $state(SIZES_GB[0]);

	// Restore-as-new.
	let restoreAsNewFor = $state<string | undefined>(undefined);
	let restoreAsNewName = $state('');

	async function refresh() {
		try {
			project = await getProject(id);
			lastUpdated = new Date();
			notFound = false;
		} catch (err) {
			if (err instanceof ApiError && err.code === 'not_found') {
				notFound = true;
				return;
			}
			toastApiError(err, 'Could not load the project.');
		}
	}

	async function refreshEvents() {
		try {
			events = (await listEvents(id)).slice().sort((a, b) => (a.ts < b.ts ? 1 : -1));
		} catch (err) {
			toastApiError(err, 'Could not load events.');
		}
	}

	async function refreshSnapshots() {
		try {
			snapshots = await listSnapshots(id);
		} catch (err) {
			toastApiError(err, 'Could not load snapshots.');
		}
	}

	async function refreshRevisions() {
		try {
			revisions = await listRevisions(id);
		} catch {
			// The config card degrades to "—" below; the config page is the
			// place that must work.
		}
	}

	async function refreshUsage() {
		if (!project) return;
		const now = new Date();
		const from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().slice(0, 10);
		const to = now.toISOString().slice(0, 10);
		try {
			const rows = await getUsage(from, to);
			const mine = rows.filter((r) => r.project_id === id);
			const monthCents = mine.reduce((sum, r) => sum + r.cost_cents, 0);
			const today = mine.find((r) => r.day === to);
			projectedMonth = projectedMonthCents(
				monthCents || project.cost_month_cents,
				today?.cost_cents ?? project.cost_today_cents
			);
		} catch {
			// The cost card still shows today/month from the project object.
		}
	}

	onMount(() => {
		const stop1 = pollWhileVisible(refresh);
		const stop2 = pollWhileVisible(refreshEvents);
		const stop3 = pollWhileVisible(refreshSnapshots);
		void refreshRevisions();
		return () => {
			stop1();
			stop2();
			stop3();
		};
	});

	$effect(() => {
		if (project) void refreshUsage();
	});

	function waitForOp(opId: string, onDone: () => void) {
		pollUntilDone(async () => {
			try {
				const op = await getOp(id, opId);
				if (op.state === 'done') {
					onDone();
					return true;
				}
				if (op.state === 'error') {
					toast.error(op.error ?? 'The operation failed.');
					opBusy = undefined;
					return true;
				}
				return false;
			} catch (err) {
				toastApiError(err);
				opBusy = undefined;
				return true;
			}
		});
	}

	async function onStart() {
		opBusy = 'start';
		startBanner = undefined;
		try {
			const { op_id } = await startProject(id);
			waitForOp(op_id, () => {
				opBusy = undefined;
				void refresh();
			});
		} catch (err) {
			opBusy = undefined;
			if (err instanceof ApiError && (err.code === 'payment_required' || err.code === 'capacity')) {
				startBanner = err.code;
				return;
			}
			toastApiError(err, 'Could not start the project.');
		}
	}

	async function onStop() {
		opBusy = 'stop';
		try {
			const { op_id } = await stopProject(id, stopSnapshotFirst);
			waitForOp(op_id, () => {
				opBusy = undefined;
				void refresh();
			});
		} catch (err) {
			opBusy = undefined;
			toastApiError(err, 'Could not stop the project.');
		}
	}

	async function onDestroy() {
		opBusy = 'destroy';
		try {
			await destroyProject(id);
			// The destroy runs on after this returns (I-166); the list shows it
			// as destroying, then under "Recently destroyed" with a Restore.
			toast.success(
				`Destroying ${project?.name}. It can be restored from "Recently destroyed" for 30 days.`
			);
			await goto(resolve('/projects'));
		} catch (err) {
			opBusy = undefined;
			toastApiError(err, 'Could not destroy the project.');
		}
	}

	async function onResize() {
		opBusy = 'resize';
		try {
			const { op_id } = await resizeProject(id, resizeTo * 1024 * 1024 * 1024);
			waitForOp(op_id, () => {
				opBusy = undefined;
				showResize = false;
				void refresh();
			});
		} catch (err) {
			opBusy = undefined;
			toastApiError(err, 'Could not resize the volume.');
		}
	}

	async function onCreateSnapshot() {
		opBusy = 'snapshot';
		try {
			const { op_id } = await createSnapshot(id);
			waitForOp(op_id, () => {
				opBusy = undefined;
				void refreshSnapshots();
			});
		} catch (err) {
			opBusy = undefined;
			toastApiError(err, 'Could not take a snapshot.');
		}
	}

	async function onRestore(snapshotId: string, asNew?: string) {
		if (!asNew && !confirm('Restoring replaces this project\'s current disk. Continue?')) return;
		opBusy = 'restore';
		try {
			const { op_id } = await restoreSnapshot(id, snapshotId, asNew);
			waitForOp(op_id, () => {
				opBusy = undefined;
				restoreAsNewFor = undefined;
				restoreAsNewName = '';
				if (asNew) toast.success(`Created ${asNew} from the snapshot.`);
				void refresh();
				void refreshSnapshots();
			});
		} catch (err) {
			opBusy = undefined;
			toastApiError(err, 'Could not restore the snapshot.');
		}
	}

	function currentRevisionStatus(): Revision | undefined {
		return revisions.find((r) => r.revision_id === project?.config_revision_id);
	}
</script>

<svelte:head>
	<title>{project ? `${project.name} — repose` : 'Project — repose'}</title>
</svelte:head>

{#if notFound}
	<PageShell title="Not found" crumbs={[{ label: 'Projects', href: resolve('/projects') }]}>
		<p class="text-sm text-zinc-500 dark:text-zinc-400">
			This project doesn't exist, or isn't yours.
		</p>
	</PageShell>
{:else if !project}
	<PageShell title="Loading…" crumbs={[{ label: 'Projects', href: resolve('/projects') }]}>
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	</PageShell>
{:else}
	<PageShell title={project.name} crumbs={[{ label: 'Projects', href: resolve('/projects') }]}>
		{#snippet action()}
			<div class="flex items-center gap-2">
				{#if project?.state === 'stopped'}
					<button type="button" class="btn" disabled={!!opBusy} onclick={onStart}>
						{opBusy === 'start' ? 'Starting…' : 'Start'}
					</button>
				{:else if project?.state === 'running'}
					<label class="check-row">
						<input type="checkbox" bind:checked={stopSnapshotFirst} />
						Snapshot on stop
					</label>
					<button type="button" class="btn" disabled={!!opBusy} onclick={onStop}>
						{opBusy === 'stop' ? 'Stopping…' : 'Stop'}
					</button>
				{/if}
			</div>
		{/snippet}

		<p class="text-sm text-zinc-600 dark:text-zinc-400">
			<StateDot state={project.state} />
			<span class="mx-1.5">·</span>
			{project.class}
			{#if project.state === 'running'}
				<span class="mx-1.5">·</span>
				up {uptime(project.started_at)}
			{/if}
		</p>

		{#if startBanner === 'payment_required'}
			<div class="banner banner--warn mt-4">
				A card on file is required to start a project. <a href={resolve('/billing')} class="link"
					>Add one in Billing</a
				>.
			</div>
		{:else if startBanner === 'capacity'}
			<div class="banner banner--warn mt-4">No capacity right now, try again in a few minutes.</div>
		{/if}

		<div class="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="card">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Connect</h2>
				<code class="codeblock mt-3 block px-3 py-2 text-sm">repose run</code>
				<code class="codeblock mt-2 block px-3 py-2 text-sm">ssh {project.slug}.repose</code>
				<p class="mt-2 text-xs text-zinc-400 dark:text-zinc-500">
					{normalizeRemoteDisplay(project.remote_url)}
				</p>
			</div>

			<div class="card">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Signals</h2>
				{#if project.signals}
					<dl class="mt-3 space-y-1 text-sm">
						<div class="flex justify-between">
							<dt class="text-zinc-500 dark:text-zinc-400">SSH sessions</dt>
							<dd>{project.signals.ssh_sessions}</dd>
						</div>
						<div class="flex justify-between">
							<dt class="text-zinc-500 dark:text-zinc-400">tmux clients</dt>
							<dd>{project.signals.tmux_clients}</dd>
						</div>
						{#if project.signals.agents.length === 0}
							<div class="flex justify-between">
								<dt class="text-zinc-500 dark:text-zinc-400">Agents</dt>
								<dd>none</dd>
							</div>
						{:else}
							{#each project.signals.agents as a (a.window)}
								<div class="flex justify-between">
									<dt class="text-zinc-500 dark:text-zinc-400">{a.agent} ({a.window})</dt>
									<dd>{a.state}</dd>
								</div>
							{/each}
						{/if}
					</dl>
				{:else}
					<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">Not running.</p>
				{/if}
				{#if lastUpdated}
					<p class="mt-3 text-xs text-zinc-400 dark:text-zinc-500">
						Updated {relativeTime(lastUpdated.toISOString())}
					</p>
				{/if}
			</div>

			<div class="card">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Cost</h2>
				<dl class="mt-3 space-y-1 text-sm">
					<div class="flex justify-between">
						<dt class="text-zinc-500 dark:text-zinc-400">Today</dt>
						<dd>{money(project.cost_today_cents)}</dd>
					</div>
					<div class="flex justify-between">
						<dt class="text-zinc-500 dark:text-zinc-400">This month</dt>
						<dd>{money(project.cost_month_cents)}</dd>
					</div>
					<div class="flex justify-between">
						<dt class="text-zinc-500 dark:text-zinc-400">Projected month</dt>
						<dd>{projectedMonth !== undefined ? money(projectedMonth) : '—'}</dd>
					</div>
				</dl>
			</div>

			<div class="card">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Disk</h2>
				<p class="mt-3 text-sm">
					{project.disk_used_bytes !== undefined ? gb(project.disk_used_bytes) : '—'} / {gb(
						project.volume_bytes
					)}
				</p>
				{#if !showResize}
					<button type="button" class="btn-ghost mt-2 px-0" onclick={() => (showResize = true)}
						>Resize…</button
					>
				{:else}
					{@const currentVolumeBytes = project.volume_bytes}
					<div class="mt-2 flex items-center gap-2">
						<select class="field" bind:value={resizeTo}>
							{#each SIZES_GB.filter((s) => s * 1024 * 1024 * 1024 > currentVolumeBytes) as s (s)}
								<option value={s}>{s} GB</option>
							{/each}
						</select>
						<button type="button" class="btn" disabled={!!opBusy} onclick={onResize}>
							{opBusy === 'resize' ? 'Resizing…' : 'Grow'}
						</button>
						<button type="button" class="btn-ghost" onclick={() => (showResize = false)}
							>Cancel</button
						>
					</div>
				{/if}
			</div>

			<div class="card">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Last build</h2>
				{#if currentRevisionStatus()}
					{@const rev = currentRevisionStatus()}
					<p class="mt-3 text-sm">
						<span
							class="badge"
							class:badge--new={rev?.status === 'applied'}
							class:badge--error={rev?.status === 'failed'}>{rev?.status}</span
						>
					</p>
				{:else}
					<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">base {project.base_version}</p>
				{/if}
				<a href={resolve('/projects/[id]/config', { id })} class="link mt-2 inline-block text-sm">View config</a>
			</div>

			<div class="card sm:col-span-2">
				<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Events</h2>
				{#if events.length === 0}
					<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">No events yet.</p>
				{:else}
					<ul class="mt-2">
						{#each events.slice(0, 20) as e (e.id)}
							<li class="row flex items-start justify-between gap-4 text-sm">
								<span>
									{#if e.agent}<span class="badge">{e.agent}</span>{/if}
									<span class="ml-2">{e.summary}</span>
								</span>
								<span class="shrink-0 text-xs text-zinc-400 dark:text-zinc-500"
									>{relativeTime(e.ts)}</span
								>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="card sm:col-span-2">
				<div class="flex items-center justify-between">
					<h2 class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Snapshots</h2>
					<button type="button" class="btn-ghost" disabled={!!opBusy} onclick={onCreateSnapshot}>
						{opBusy === 'snapshot' ? 'Snapshotting…' : 'Create'}
					</button>
				</div>
				{#if snapshots.length === 0}
					<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">No snapshots yet.</p>
				{:else}
					<ul class="mt-2">
						{#each snapshots as s (s.id)}
							<li class="row flex items-center justify-between gap-4 text-sm">
								<span>{dateTime(s.created_at)} · {gb(s.bytes)} · {s.reason}</span>
								<span class="flex shrink-0 items-center gap-3">
									{#if restoreAsNewFor === s.id}
										<input
											class="field w-40 py-1"
											placeholder="new project name"
											bind:value={restoreAsNewName}
										/>
										<button
											type="button"
											class="btn-ghost"
											disabled={!restoreAsNewName || !!opBusy}
											onclick={() => onRestore(s.id, restoreAsNewName)}>Restore as new</button
										>
										<button
											type="button"
											class="btn-ghost"
											onclick={() => (restoreAsNewFor = undefined)}>Cancel</button
										>
									{:else}
										<button
											type="button"
											class="btn-ghost"
											disabled={!!opBusy}
											onclick={() => onRestore(s.id)}>Restore</button
										>
										<button
											type="button"
											class="btn-ghost"
											onclick={() => (restoreAsNewFor = s.id)}>Restore as new…</button
										>
									{/if}
								</span>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>

		<div class="form-section">
			<h2 class="text-sm font-semibold text-red-700 dark:text-red-400">Destroy</h2>
			<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
				Deletes the volume. The last snapshot is kept 30 days.
			</p>
			<div class="mt-3">
				<ConfirmType
					word={project.slug}
					label="Destroy"
					disabled={!!opBusy}
					onconfirm={onDestroy}
				/>
			</div>
		</div>
	</PageShell>
{/if}
