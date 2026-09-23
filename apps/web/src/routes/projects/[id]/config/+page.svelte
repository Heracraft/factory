<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import {
		getConfig,
		putConfig,
		getCatalog,
		listRevisions,
		applyRevision,
		getProject,
		patchProject,
		opLogUrl,
		getOp
	} from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { dateTime } from '$lib/format';
	import { toggleSelection, groupCatalog, buildMenuSelection } from '$lib/menuSelection';
	import PageShell from '$lib/components/PageShell.svelte';
	import NixEditor from '$lib/components/NixEditor.svelte';
	import type { CatalogItem, Config, Project, Revision } from '$lib/api/types';

	const id = page.params.id as string;

	let project = $state<Project | undefined>(undefined);
	let config = $state<Config | undefined>(undefined);
	let catalog = $state<CatalogItem[]>([]);
	let revisions = $state<Revision[]>([]);
	let activeTab = $state<'menu' | 'nix'>('nix');
	let search = $state('');

	// Menu tab state.
	let selectedPackages = $state<Set<string>>(new Set());
	let selectedServices = $state<Set<string>>(new Set());
	let menuOptions = $state<Record<string, string>>({});

	// Nix tab state.
	let fragmentText = $state('');
	let nixOverrideEditable = $state(false);
	let nixReadonly = $derived(!!config?.menu && !nixOverrideEditable);

	// Build/apply state, shared by both tabs.
	let applying = $state(false);
	let buildLines = $state<string[]>([]);
	let buildDone = $state(false);
	let buildError = $state<string | undefined>(undefined);
	let currentOpId = $state<string | undefined>(undefined);
	let eventSource: EventSource | undefined;
	let logEl = $state<HTMLPreElement | undefined>(undefined);

	function fragmentErrorLine(message: string | undefined): number | undefined {
		if (!message) return undefined;
		const m = /fragment\.nix:(\d+):/.exec(message);
		return m ? Number(m[1]) : undefined;
	}

	async function load() {
		try {
			[project, config, catalog, revisions] = await Promise.all([
				getProject(id),
				getConfig(id),
				getCatalog(),
				listRevisions(id)
			]);
			activeTab = config?.menu ? 'menu' : 'nix';
			fragmentText = config?.fragment ?? '';
			selectedPackages = new Set(config?.menu?.packages ?? []);
			selectedServices = new Set(config?.menu?.services ?? []);
			menuOptions = { ...(config?.menu?.options ?? {}) };
		} catch (err) {
			toastApiError(err, 'Could not load the config.');
		}
	}

	onMount(load);
	onDestroy(() => eventSource?.close());

	let groups = $derived(groupCatalog(catalog, search));

	function toggleItem(item: CatalogItem, checked: boolean) {
		if (item.kind === 'service') {
			selectedServices = toggleSelection(selectedServices, item.id, checked);
		} else {
			selectedPackages = toggleSelection(selectedPackages, item.id, checked);
		}
	}

	function isSelected(item: CatalogItem): boolean {
		return item.kind === 'service' ? selectedServices.has(item.id) : selectedPackages.has(item.id);
	}

	function startBuild(opId: string) {
		currentOpId = opId;
		buildLines = [];
		buildDone = false;
		buildError = undefined;
		eventSource?.close();
		void (async () => {
			const url = await opLogUrl(id, opId);
			const es = new EventSource(url);
			eventSource = es;
			es.onmessage = (ev) => {
				const data = JSON.parse(ev.data) as { seq: number; line: string };
				buildLines = [...buildLines, data.line];
				queueMicrotask(() => logEl?.scrollTo(0, logEl.scrollHeight));
			};
			es.addEventListener('done', async () => {
				es.close();
				buildDone = true;
				applying = false;
				try {
					const op = await getOp(id, opId);
					if (op.state === 'error') buildError = op.error;
				} catch {
					// The revision list below still shows failed/applied.
				}
				await load();
			});
			es.onerror = () => {
				es.close();
				buildDone = true;
				applying = false;
			};
		})();
	}

	async function applyMenu() {
		applying = true;
		const menu = buildMenuSelection(selectedPackages, selectedServices, menuOptions);
		try {
			const { op_id } = await putConfig(id, { menu });
			startBuild(op_id);
		} catch (err) {
			applying = false;
			toastApiError(err, 'Could not apply the menu selection.');
		}
	}

	async function applyNix() {
		applying = true;
		try {
			const { op_id } = await putConfig(id, { fragment: fragmentText });
			startBuild(op_id);
		} catch (err) {
			applying = false;
			toastApiError(err, 'Could not apply the fragment.');
		}
	}

	async function reapply(rev: Revision) {
		try {
			const { op_id } = await applyRevision(id, rev.revision_id);
			startBuild(op_id);
		} catch (err) {
			toastApiError(err, 'Could not re-apply that revision.');
		}
	}

	function editAsNix() {
		nixOverrideEditable = true;
		activeTab = 'nix';
	}

	async function toggleHold() {
		if (!project) return;
		const next = !project.hold_base_updates;
		try {
			project = await patchProject(id, { hold_base_updates: next });
		} catch (err) {
			toastApiError(err, 'Could not change the hold flag.');
		}
	}
</script>

<svelte:head>
	<title>{project ? `${project.name} config — repose` : 'Config — repose'}</title>
</svelte:head>

<PageShell
	title="Config"
	crumbs={[
		{ label: 'Projects', href: resolve('/projects') },
		{ label: project?.name ?? '…', href: resolve('/projects/[id]', { id }) }
	]}
>
	{#if !project || !config}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else}
		<div class="flex gap-6 border-b border-zinc-200 text-sm dark:border-zinc-800">
			<button
				type="button"
				class="border-b-2 py-2 {activeTab === 'menu'
					? 'border-blue-700 font-medium text-blue-700 dark:border-blue-400 dark:text-blue-400'
					: 'border-transparent text-zinc-500 dark:text-zinc-400'}"
				onclick={() => (activeTab = 'menu')}>Menu</button
			>
			<button
				type="button"
				class="border-b-2 py-2 {activeTab === 'nix'
					? 'border-blue-700 font-medium text-blue-700 dark:border-blue-400 dark:text-blue-400'
					: 'border-transparent text-zinc-500 dark:text-zinc-400'}"
				onclick={() => (activeTab = 'nix')}>Nix</button
			>
		</div>

		{#if activeTab === 'menu'}
			<div class="mt-4">
				{#if config.menu === null || config.menu === undefined}
					<p class="banner banner--warn">
						This project's config was edited by hand; applying from the menu will replace it.
					</p>
				{/if}
				<input
					type="search"
					class="field w-full"
					placeholder="Search packages and services…"
					bind:value={search}
				/>
				{#each [...groups.entries()] as [group, items] (group)}
					<div class="form-section">
						<h2 class="font-display text-xl font-semibold">{group}</h2>
						{#each items as item (item.id)}
							<label class="check-list-row">
								<input
									type="checkbox"
									checked={isSelected(item)}
									onchange={(e) => toggleItem(item, e.currentTarget.checked)}
								/>
								<span class="flex-1">
									<span class="block text-sm font-medium">{item.label}</span>
									<span class="block text-sm text-zinc-500 dark:text-zinc-400"
										>{item.description}</span
									>
									{#if item.options?.length && isSelected(item)}
										<select
											class="field mt-2 w-48"
											value={menuOptions[item.id] ?? item.options[0].default}
											onchange={(e) => (menuOptions[item.id] = e.currentTarget.value)}
										>
											{#each item.options[0].values as v (v)}
												<option value={v}>{v}</option>
											{/each}
										</select>
									{/if}
								</span>
							</label>
						{/each}
					</div>
				{/each}
				<div class="form-section">
					<button type="button" class="btn" disabled={applying} onclick={applyMenu}>
						{applying ? 'Applying…' : 'Apply'}
					</button>
				</div>
			</div>
		{:else}
			<div class="mt-4">
				{#if nixReadonly}
					<p class="banner banner--warn">
						This project is managed by the menu. Editing here takes over from the menu.
						<button type="button" class="link" onclick={editAsNix}>Edit as Nix</button>
					</p>
				{/if}
				<NixEditor
					bind:value={fragmentText}
					readonly={nixReadonly}
					errorLine={fragmentErrorLine(buildError)}
				/>
				<div class="form-section">
					<button type="button" class="btn" disabled={applying || nixReadonly} onclick={applyNix}>
						{applying ? 'Applying…' : 'Apply'}
					</button>
				</div>
			</div>
		{/if}

		{#if currentOpId}
			<div class="form-section">
				<h2 class="font-display text-xl font-semibold">Build log</h2>
				<pre class="codeblock mt-2 h-56 overflow-y-auto" bind:this={logEl}>{buildLines.join(
						'\n'
					)}</pre>
				{#if buildDone && buildError}
					<div class="banner banner--error mt-3">
						<p class="font-mono text-xs whitespace-pre-wrap">{buildError}</p>
					</div>
				{:else if buildDone}
					<p class="mt-3 text-sm text-emerald-700 dark:text-emerald-400">Applied.</p>
				{/if}
			</div>
		{/if}

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Base updates</h2>
			<label class="check-row mt-2">
				<input type="checkbox" checked={project.hold_base_updates} onchange={toggleHold} />
				Hold base updates (currently on {project.base_version})
			</label>
		</div>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Revisions</h2>
			{#if revisions.length === 0}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">No revisions yet.</p>
			{:else}
				<ul class="mt-2">
					{#each revisions as rev (rev.revision_id)}
						<li class="row flex items-center justify-between gap-4 text-sm">
							<span>
								{dateTime(rev.created_at)}
								<span
									class="badge ml-2"
									class:badge--new={rev.status === 'applied'}
									class:badge--error={rev.status === 'failed'}>{rev.status}</span
								>
							</span>
							{#if rev.status !== 'building'}
								<button type="button" class="btn-ghost" onclick={() => reapply(rev)}
									>Re-apply</button
								>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</PageShell>
