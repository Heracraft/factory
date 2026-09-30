<script lang="ts">
	// A destructive action's confirm control: the button stays disabled until
	// the exact word is typed, per 08-dashboard.md 5.2 ("Destroy with confirm
	// typing the slug") and 6 ("Destroy typed wrong -> button disabled until
	// the slug matches exactly"). It is the one pattern for an action that
	// cannot be undone: destroying a project, deleting the account, and
	// restoring a snapshot over a project's disk all use it.
	let {
		word,
		label,
		onconfirm,
		disabled = false,
		busy = false,
		busyLabel,
		oncancel
	}: {
		word: string;
		label: string;
		onconfirm: () => void;
		disabled?: boolean;
		/** The action is under way: the button says so and stays disabled. */
		busy?: boolean;
		busyLabel?: string;
		/** When given, a Cancel beside the button closes the panel around it. */
		oncancel?: () => void;
	} = $props();

	let typed = $state('');
	let ready = $derived(!disabled && !busy && typed === word);
</script>

<!-- A form so Enter confirms once the word matches, the same as a click. -->
<form
	class="flex flex-col gap-2 sm:flex-row sm:items-center"
	onsubmit={(e) => {
		e.preventDefault();
		if (ready) onconfirm();
	}}
>
	<input
		class="field w-full sm:w-56"
		placeholder={`Type "${word}" to confirm`}
		bind:value={typed}
		autocomplete="off"
		spellcheck="false"
		aria-label={`Type ${word} to confirm`}
	/>
	<div class="flex items-center gap-2">
		<button type="submit" class="btn-danger" disabled={!ready}>
			{busy && busyLabel ? busyLabel : label}
		</button>
		{#if oncancel}
			<button type="button" class="btn-ghost" onclick={oncancel}>Cancel</button>
		{/if}
	</div>
</form>
