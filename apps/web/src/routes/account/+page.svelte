<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { getMe, deleteMe } from '$lib/api/client';
	import { toastApiError } from '$lib/api/toast';
	import { signOut } from '$lib/auth.svelte';
	import PageShell from '$lib/components/PageShell.svelte';
	import ConfirmType from '$lib/components/ConfirmType.svelte';
	import type { Me } from '$lib/api/types';

	let me = $state<Me | undefined>(undefined);
	let deleting = $state(false);

	onMount(async () => {
		try {
			me = await getMe();
		} catch (err) {
			toastApiError(err, 'Could not load your account.');
		}
	});

	async function onDelete() {
		deleting = true;
		try {
			await deleteMe();
			toast.success('Account cancellation started. Everything is deleted in 30 days.');
			await signOut();
		} catch (err) {
			deleting = false;
			toastApiError(err, 'Could not start account deletion.');
		}
	}
</script>

<svelte:head>
	<title>Account — repose</title>
</svelte:head>

<PageShell title="Account" width="form">
	{#if !me}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else}
		<dl class="space-y-2 text-sm">
			<div class="flex justify-between">
				<dt class="text-zinc-500 dark:text-zinc-400">Handle</dt>
				<dd class="font-mono">{me.handle}</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-zinc-500 dark:text-zinc-400">Email</dt>
				<dd>{me.email}</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-zinc-500 dark:text-zinc-400">GitHub</dt>
				<dd>{me.github_login}</dd>
			</div>
		</dl>

		<div class="form-section">
			<h2 class="text-xl font-semibold text-red-700 dark:text-red-400">Delete account</h2>
			<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
				Stops every environment at once. Everything, including snapshots, is deleted 30 days later.
			</p>
			<div class="mt-3">
				<ConfirmType
					word={me.handle}
					label="Delete account"
					disabled={deleting}
					onconfirm={onDelete}
				/>
			</div>
		</div>
	{/if}
</PageShell>
