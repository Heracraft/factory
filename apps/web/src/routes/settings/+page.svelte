<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { getMe, patchMe, notifyTest } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import PageShell from '$lib/components/PageShell.svelte';
	import type { Me } from '$lib/api/types';

	let me = $state<Me | undefined>(undefined);
	let tz = $state('');
	let emailOn = $state(true);
	let ntfyUrl = $state('');
	let saving = $state(false);
	let testAvailable = $state(true);
	let testing = $state(false);

	const timezones =
		typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];

	async function load() {
		try {
			me = await getMe();
			tz = me.tz || Intl.DateTimeFormat().resolvedOptions().timeZone;
			emailOn = me.notify?.email ?? true;
			ntfyUrl = me.notify?.ntfy_url ?? '';
		} catch (err) {
			toastApiError(err, 'Could not load settings.');
		}
	}

	onMount(load);

	async function save() {
		saving = true;
		try {
			me = await patchMe({ tz, notify: { email: emailOn, ntfy_url: ntfyUrl || null } });
			toast.success('Settings saved.');
		} catch (err) {
			toastApiError(err, 'Could not save settings.');
		} finally {
			saving = false;
		}
	}

	async function sendTest() {
		testing = true;
		try {
			const result = await notifyTest();
			if (result.email === 'ok' || result.ntfy === 'ok') toast.success('Test notification sent.');
			else toast.error('The test notification failed on every channel.');
		} catch (err) {
			if (err instanceof ApiError && err.code === 'not_found') testAvailable = false;
			else toastApiError(err, 'Could not send a test notification.');
		} finally {
			testing = false;
		}
	}
</script>

<svelte:head>
	<title>Settings — repose</title>
</svelte:head>

<PageShell title="Settings" width="form">
	{#if !me}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else}
		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Timezone</h2>
			{#if timezones.length}
				<select class="field mt-2 w-full sm:w-72" bind:value={tz}>
					{#each timezones as z (z)}
						<option value={z}>{z}</option>
					{/each}
				</select>
			{:else}
				<input class="field mt-2 w-full sm:w-72" bind:value={tz} />
			{/if}
		</div>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Notifications</h2>
			<label class="check-row mt-2">
				<input type="checkbox" bind:checked={emailOn} />
				Email notifications
			</label>
			<div class="mt-3">
				<label for="ntfy-url" class="mb-1 block text-sm text-zinc-700 dark:text-zinc-300"
					>ntfy URL</label
				>
				<input
					id="ntfy-url"
					class="field w-full"
					placeholder="https://ntfy.sh/your-topic"
					bind:value={ntfyUrl}
				/>
			</div>
			<div class="mt-3">
				{#if testAvailable}
					<button type="button" class="btn-ghost px-0" disabled={testing} onclick={sendTest}>
						{testing ? 'Sending…' : 'Send test'}
					</button>
				{:else}
					<span class="text-sm text-zinc-400 dark:text-zinc-500">Test not available yet</span>
				{/if}
			</div>
		</div>

		<div class="form-section">
			<button type="button" class="btn" disabled={saving} onclick={save}>
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Install</h2>
			<code class="codeblock mt-2 block px-3 py-2 text-sm"
				>curl -fsSL https://repose.herakraft.co/install.sh | sh</code
			>
			<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
				The CLI writes <code class="font-mono">~/.ssh/repose/config</code> and includes it from your
				main SSH config, so <code class="font-mono">ssh &lt;slug&gt;.repose</code> works once you've
				run <code class="font-mono">repose login</code>.
			</p>
		</div>
	{/if}
</PageShell>
