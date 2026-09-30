<script lang="ts" module>
	import { errorText } from '$lib/api/errors';

	/**
	 * The sentence a failed first load shows: errorText, the one toastApiError
	 * uses, so the banner and the toast never disagree about one failure.
	 */
	export function loadErrorText(err: unknown, fallback: string): string {
		return errorText(err, fallback);
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';

	/*
	 * The one wrapper for a page's first load. While it runs the page says
	 * "Loading…"; if it fails the page says why and offers Retry, which
	 * re-runs the same load. Without this a page whose first fetch failed
	 * sat on "Loading…" for good, and the toast that named the error was
	 * gone after a few seconds.
	 *
	 * Only the first load belongs here. A later refresh that fails keeps
	 * the content already on screen and reports through a toast, so the
	 * page never blanks out under someone who is reading it.
	 */
	let {
		status,
		onretry,
		label = 'Loading…',
		error,
		children
	}: {
		status: 'loading' | 'failed' | 'ready';
		onretry: () => void | Promise<void>;
		/** What the page says while the load runs. */
		label?: string;
		/** The reason the load failed, as the api or network put it. */
		error?: string;
		children?: Snippet;
	} = $props();

	let retrying = $state(false);

	async function retry() {
		if (retrying) return;
		retrying = true;
		try {
			await onretry();
		} finally {
			retrying = false;
		}
	}
</script>

{#if status === 'ready'}
	{@render children?.()}
{:else if status === 'failed'}
	<!-- role=alert so a screen reader announces the failure, which arrives
	     after the page has already said it is loading. -->
	<div class="banner banner--error flex flex-wrap items-center justify-between gap-3" role="alert">
		<p>
			{error ?? 'This page could not be loaded.'}
		</p>
		<!-- aria-disabled rather than disabled while the retry runs: a
		     disabled button loses focus (the browser moves it to <body>), and
		     the person who pressed Retry with the keyboard would start over
		     from the top of the page. It looks disabled and ignores presses. -->
		<button
			type="button"
			class="btn-quiet btn--sm"
			aria-disabled={retrying ? 'true' : undefined}
			onclick={retry}>{retrying ? 'Retrying…' : 'Retry'}</button
		>
	</div>
{:else}
	<!-- role=status is polite: the word is read once, and the content that
	     replaces it is not interrupted. -->
	<p class="text-sm text-ink-muted" role="status">{label}</p>
{/if}
