<!-- The name for a project restored from a snapshot, asked in place: under
     a row in "Recently destroyed" (DECISIONS I-167) and under a snapshot on
     the project page ("Restore as new"). One component so both carry the
     same visible label, the same error wiring and the same wrap at phone
     width; the two copies had drifted apart before. The form opens in
     place of the button that asked for it, so it takes focus into its
     field; the caller puts focus back on that button when it closes. -->
<script lang="ts">
	import { focusOnMount } from '$lib/focus';

	let {
		id,
		value = $bindable(''),
		error,
		busy = false,
		submitLabel = 'Restore',
		busyLabel = 'Restoring…',
		onsubmit,
		oncancel
	}: {
		/** Unique on the page; the field and its error hang off it. */
		id: string;
		value?: string;
		/** Why the api refused the name last time. */
		error?: string;
		busy?: boolean;
		submitLabel?: string;
		busyLabel?: string;
		onsubmit: (name: string) => void;
		oncancel: () => void;
	} = $props();

	let fieldId = $derived(`restore-name-${id}`);
	let errorId = $derived(`restore-error-${id}`);
</script>

<form
	class="mt-3"
	onsubmit={(e) => {
		e.preventDefault();
		if (value.trim() !== '' && !busy) onsubmit(value.trim());
	}}
>
	<label for={fieldId} class="block text-sm text-ink-muted">Name for the restored project</label>
	<!-- flex-wrap and a field that may shrink: at 390px the buttons drop
	     under the field instead of pushing the page sideways. -->
	<div class="mt-1.5 flex flex-wrap items-center gap-2">
		<input
			id={fieldId}
			class="field w-full min-w-0 sm:w-56"
			bind:value
			autocomplete="off"
			spellcheck="false"
			aria-invalid={error ? 'true' : undefined}
			aria-describedby={error ? errorId : undefined}
			use:focusOnMount
		/>
		<button type="submit" class="btn" disabled={busy || value.trim() === ''}
			>{busy ? busyLabel : submitLabel}</button
		>
		<button type="button" class="btn-ghost" onclick={oncancel}>Cancel</button>
	</div>
	{#if error}
		<p id={errorId} class="field-error">{error}</p>
	{/if}
</form>
