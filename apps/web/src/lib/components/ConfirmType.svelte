<script lang="ts">
	// A destructive action's confirm control: the button stays disabled until
	// the exact word is typed, per 08-dashboard.md 5.2 ("Destroy with confirm
	// typing the slug") and 6 ("Destroy typed wrong -> button disabled until
	// the slug matches exactly").
	let {
		word,
		label,
		onconfirm,
		disabled = false
	}: {
		word: string;
		label: string;
		onconfirm: () => void;
		disabled?: boolean;
	} = $props();

	let typed = $state('');
</script>

<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
	<input
		class="field w-full sm:w-56"
		placeholder={`Type "${word}" to confirm`}
		bind:value={typed}
		aria-label={`Type ${word} to confirm`}
	/>
	<button
		type="button"
		class="btn-danger"
		disabled={disabled || typed !== word}
		onclick={() => onconfirm()}
	>
		{label}
	</button>
</div>
