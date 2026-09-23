<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { getProject, listSecrets, putSecret, deleteSecret } from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { encodeBase64 } from '$lib/base64';
	import { dateTime } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import type { Project, SecretMeta } from '$lib/api/types';

	const id = page.params.id as string;

	const NAME_RE = /^[A-Z][A-Z0-9_]{0,63}$/;
	const RESERVED = new Set([
		'ssh_host_ed25519_key',
		'ssh_host_ed25519_key-cert.pub',
		'user_ca.pub'
	]);
	const MAX_BYTES = 64 * 1024;

	let project = $state<Project | undefined>(undefined);
	let secrets = $state<SecretMeta[] | undefined>(undefined);

	let name = $state('');
	let value = $state('');
	let fileInput = $state<HTMLInputElement | undefined>(undefined);
	let nameError = $state<string | undefined>(undefined);
	let valueError = $state<string | undefined>(undefined);
	let saving = $state(false);
	let deleting = $state<string | undefined>(undefined);

	async function load() {
		try {
			[project, secrets] = await Promise.all([getProject(id), listSecrets(id)]);
		} catch (err) {
			toastApiError(err, 'Could not load secrets.');
		}
	}

	onMount(load);

	function validateName(n: string): string | undefined {
		if (RESERVED.has(n)) return "This name is reserved for the guest's SSH host material.";
		if (!NAME_RE.test(n))
			return 'Must match [A-Z][A-Z0-9_]{0,63} — uppercase letters, digits and underscore.';
		return undefined;
	}

	async function onAdd() {
		nameError = validateName(name);
		let base64: string;
		if (fileInput?.files?.length) {
			const file = fileInput.files[0];
			if (file.size > MAX_BYTES) {
				valueError = 'Secret values are limited to 64 KB.';
			}
			base64 = encodeBase64(await file.arrayBuffer());
		} else {
			if (new Blob([value]).size > MAX_BYTES) {
				valueError = 'Secret values are limited to 64 KB.';
			}
			base64 = encodeBase64(value);
		}
		if (nameError || valueError) return;

		saving = true;
		try {
			await putSecret(id, name, base64);
			toast.success(`Stored ${name}.`);
			name = '';
			value = '';
			if (fileInput) fileInput.value = '';
			await load();
		} catch (err) {
			toastApiError(err, 'Could not store the secret.');
		} finally {
			saving = false;
		}
	}

	async function onDelete(n: string) {
		if (
			!confirm(`Remove ${n}? Running processes that already read it keep their copy until restart.`)
		)
			return;
		deleting = n;
		try {
			await deleteSecret(id, n);
			await load();
		} catch (err) {
			toastApiError(err, 'Could not remove the secret.');
		} finally {
			deleting = undefined;
		}
	}
</script>

<svelte:head>
	<title>{project ? `${project.name} secrets — repose` : 'Secrets — repose'}</title>
</svelte:head>

<PageShell
	title="Secrets"
	width="form"
	crumbs={[
		{ label: 'Projects', href: resolve('/projects') },
		{ label: project?.name ?? '…', href: resolve('/projects/[id]', { id }) }
	]}
>
	{#if secrets === undefined}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else}
		{#if secrets.length === 0}
			<p class="text-sm text-zinc-500 dark:text-zinc-400">No secrets yet.</p>
		{:else}
			<ul>
				{#each secrets as s (s.name)}
					<li class="row flex items-center justify-between gap-4">
						<span class="font-mono text-sm">{s.name}</span>
						<span class="flex items-center gap-4">
							<span class="text-xs text-zinc-400 dark:text-zinc-500"
								>updated {dateTime(s.updated_at)}</span
							>
							<button
								type="button"
								class="btn-ghost-danger"
								disabled={deleting === s.name}
								onclick={() => onDelete(s.name)}>Delete</button
							>
						</span>
					</li>
				{/each}
			</ul>
		{/if}

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Add a secret</h2>
			<div class="mt-3 flex flex-col gap-3">
				<div>
					<input
						class="field w-full font-mono"
						placeholder="NAME"
						bind:value={name}
						oninput={() => (nameError = undefined)}
					/>
					{#if nameError}<p class="field-error">{nameError}</p>{/if}
				</div>
				<div>
					<textarea
						class="field w-full"
						placeholder="Value"
						bind:value
						oninput={() => (valueError = undefined)}
					></textarea>
					<input type="file" bind:this={fileInput} class="mt-2 text-sm" />
					{#if valueError}<p class="field-error">{valueError}</p>{/if}
				</div>
				<div>
					<button type="button" class="btn" disabled={saving || !name} onclick={onAdd}>
						{saving ? 'Storing…' : 'Add'}
					</button>
				</div>
			</div>
		</div>
	{/if}
</PageShell>
