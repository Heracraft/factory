<script lang="ts">
	import { onMount } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { getMe, patchMe, notifyTest } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import PageShell from '$lib/components/PageShell.svelte';
	import type { Me } from '$lib/api/types';

	// The timezone and the email toggle save the moment they change; the
	// ntfy URL is typed, so it has its own Save next to the field
	// (DECISIONS I-332).
	let me = $state<Me | undefined>(undefined);
	let tz = $state('');
	let savedTz = '';
	// The zone the account had when the page loaded.
	let loadedTz = $state('');
	let emailOn = $state(true);
	let ntfyUrl = $state('');
	let savedNtfyUrl = $state('');
	let savingNtfy = $state(false);
	let testAvailable = $state(true);
	let testing = $state(false);

	let ntfyDirty = $derived(ntfyUrl.trim() !== savedNtfyUrl);

	const supported =
		typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];
	// The browser's list leaves out some names the api accepts, UTC among
	// them; the account's own zone is always an option, or the select shows
	// blank and the next change would save a zone the user never picked.
	let timezones = $derived(
		supported.length && loadedTz && !supported.includes(loadedTz)
			? [loadedTz, ...supported]
			: supported
	);

	async function load() {
		try {
			me = await getMe();
			tz = me.tz || Intl.DateTimeFormat().resolvedOptions().timeZone;
			savedTz = tz;
			loadedTz = tz;
			emailOn = me.notify?.email ?? true;
			ntfyUrl = me.notify?.ntfy_url ?? '';
			savedNtfyUrl = ntfyUrl;
		} catch (err) {
			toastApiError(err, 'Could not load settings.');
			return;
		}
		// An account that never set a timezone shows this browser's; with
		// no Save button to press any more, store it so what the page shows
		// is what the account has. Quietly: nothing the user did.
		if (!me.tz && tz) {
			try {
				me = await patchMe({ tz });
			} catch {
				// Left unset; the next change of the select saves it.
			}
		}
	}

	onMount(load);

	beforeNavigate(({ cancel, type }) => {
		if (!ntfyDirty) return;
		// A tab close or reload gets the browser's own prompt from cancel();
		// a link inside the dashboard asks here.
		if (type === 'leave' || !confirm('The ntfy URL is not saved. Leave anyway?')) cancel();
	});

	async function saveTz() {
		const next = tz;
		try {
			me = await patchMe({ tz: next });
			savedTz = next;
			toast.success(`Timezone set to ${next}.`);
		} catch (err) {
			tz = savedTz;
			toastApiError(err, 'Could not save the timezone.');
		}
	}

	async function saveEmail() {
		const next = emailOn;
		try {
			me = await patchMe({ notify: { email: next } });
			toast.success(next ? 'Email notifications on.' : 'Email notifications off.');
		} catch (err) {
			emailOn = !next;
			toastApiError(err, 'Could not change email notifications.');
		}
	}

	async function saveNtfy(e: SubmitEvent) {
		e.preventDefault();
		const next = ntfyUrl.trim();
		savingNtfy = true;
		try {
			me = await patchMe({ notify: { ntfy_url: next || null } });
			ntfyUrl = next;
			savedNtfyUrl = next;
			toast.success(next ? 'ntfy URL saved.' : 'ntfy turned off.');
		} catch (err) {
			toastApiError(err, 'Could not save the ntfy URL.');
		} finally {
			savingNtfy = false;
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
				<select
					class="field mt-2 w-full sm:w-72"
					aria-label="Timezone"
					bind:value={tz}
					onchange={saveTz}
				>
					{#each timezones as z (z)}
						<option value={z}>{z}</option>
					{/each}
				</select>
			{:else}
				<input
					class="field mt-2 w-full sm:w-72"
					aria-label="Timezone"
					bind:value={tz}
					onchange={saveTz}
				/>
			{/if}
		</div>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Notifications</h2>
			<label class="check-row mt-2">
				<input type="checkbox" bind:checked={emailOn} onchange={saveEmail} />
				Email notifications
			</label>
			<form class="mt-3" onsubmit={saveNtfy}>
				<label for="ntfy-url" class="mb-1 block text-sm text-zinc-700 dark:text-zinc-300"
					>ntfy URL</label
				>
				<div class="flex gap-2">
					<input
						id="ntfy-url"
						class="field min-w-0 flex-1"
						placeholder="https://ntfy.sh/your-topic"
						bind:value={ntfyUrl}
					/>
					<button type="submit" class="btn shrink-0" disabled={savingNtfy || !ntfyDirty}>
						{savingNtfy ? 'Saving…' : 'Save'}
					</button>
				</div>
				{#if ntfyDirty}
					<p class="mt-1.5 text-sm text-zinc-500 dark:text-zinc-400">Not saved yet.</p>
				{/if}
			</form>
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
