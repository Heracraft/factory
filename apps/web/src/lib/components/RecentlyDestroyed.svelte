<!-- "Recently destroyed" on /projects (DECISIONS I-167): every destroyed
     project that still has a snapshot, when that snapshot goes, and a
     Restore that brings it back as a new project. The CLI's `repose
     projects --destroyed` and `repose restore NAME` are the same list and
     the same route. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { restoreProject } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { dateTime, relativeTime } from '$lib/format';
	import { defaultRestoreName, timeLeft } from '$lib/destroyed';
	import type { DestroyedProject } from '$lib/api/types';

	let {
		destroyed,
		liveSlugs,
		onrestored
	}: {
		destroyed: DestroyedProject[];
		liveSlugs: string[];
		onrestored?: () => void;
	} = $props();

	// The row whose name field is open, the name in it, and why the api
	// refused it last time.
	let openFor = $state<string | undefined>(undefined);
	let name = $state('');
	let nameError = $state<string | undefined>(undefined);
	let busy = $state(false);

	function open(d: DestroyedProject) {
		openFor = d.id;
		name = defaultRestoreName(d, liveSlugs);
		nameError = undefined;
	}

	function mb(bytes: number): string {
		if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
		if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
		return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
	}

	async function restore(d: DestroyedProject) {
		busy = true;
		nameError = undefined;
		try {
			const res = await restoreProject({ project_id: d.id, name: name.trim() });
			toast.success(
				`Restoring ${res.name} from its snapshot of ${dateTime(res.snapshot_created_at)}.`
			);
			openFor = undefined;
			onrestored?.();
			await goto(resolve('/projects/[id]', { id: res.project_id }));
		} catch (err) {
			if (
				err instanceof ApiError &&
				err.code === 'conflict' &&
				err.detail?.reason === 'name_taken'
			) {
				nameError = `A project called ${String(err.detail?.name ?? name)} already exists; pick another name.`;
			} else if (err instanceof ApiError && err.code === 'invalid') {
				nameError = err.message;
			} else {
				toastApiError(err, 'Could not restore the project.');
			}
		} finally {
			busy = false;
		}
	}
</script>

{#if destroyed.length > 0}
	<section class="mt-16" aria-labelledby="recently-destroyed">
		<h2 id="recently-destroyed" class="text-xl font-semibold">Recently destroyed</h2>
		<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
			Each keeps its last snapshot for 30 days. Restore it here or with
			<code>repose restore NAME</code>.
		</p>
		<ul class="mt-4 border-t border-[var(--rule-strong)]">
			{#each destroyed as d (d.id)}
				<li class="row" data-testid="destroyed-row">
					<div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
						<div class="min-w-0">
							<span class="font-medium">{d.name}</span>
							<span class="badge ml-2">{d.class}</span>
							{#if !d.name_free}
								<span class="badge ml-2">name in use</span>
							{/if}
						</div>
						{#if openFor !== d.id}
							<button type="button" class="btn-ghost" disabled={busy} onclick={() => open(d)}
								>Restore…</button
							>
						{/if}
					</div>
					<p class="mt-0.5 text-sm text-zinc-500 dark:text-zinc-400">
						Destroyed {relativeTime(d.destroyed_at)} · snapshot {dateTime(d.snapshot.created_at)}, {mb(
							d.snapshot.bytes
						)}
						{#if d.restorable_until}
							· restorable until {dateTime(d.restorable_until).slice(0, 10)} ({timeLeft(
								d.restorable_until
							)})
						{/if}
					</p>
					{#if openFor === d.id}
						<form
							class="mt-2 flex flex-wrap items-center gap-2"
							onsubmit={(e) => {
								e.preventDefault();
								void restore(d);
							}}
						>
							<label class="sr-only" for="restore-name-{d.id}">Name for the restored project</label>
							<input
								id="restore-name-{d.id}"
								class="field w-56 py-1"
								class:field--set={name !== ''}
								bind:value={name}
								aria-invalid={nameError ? 'true' : undefined}
								aria-describedby={nameError ? `restore-error-${d.id}` : undefined}
							/>
							<button type="submit" class="btn py-1" disabled={busy || name.trim() === ''}
								>{busy ? 'Restoring…' : 'Restore'}</button
							>
							<button type="button" class="btn-ghost" onclick={() => (openFor = undefined)}
								>Cancel</button
							>
						</form>
						{#if nameError}
							<p id="restore-error-{d.id}" class="field-error">{nameError}</p>
						{/if}
					{/if}
				</li>
			{/each}
		</ul>
	</section>
{/if}
