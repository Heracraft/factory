<script lang="ts">
	// The wordmark: the name in the heading face, behind the mark. The mark
	// is a lowercase r built the way the landing's shapes are built: a stem
	// in the text's ink and a quarter disc in the one blue, the letter's
	// shoulder. Flat, no stroke, so it holds at 16px and at 128px (the
	// favicon is this same drawing).
	let { size = 'base', mark = false }: { size?: 'sm' | 'base'; mark?: boolean } = $props();
</script>

<span
	class="inline-flex items-center gap-2 font-display font-semibold tracking-tight {size === 'sm'
		? 'text-base'
		: 'text-lg'}"
	>{#if mark}<svg
			viewBox="0 0 100 100"
			class="{size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'} shrink-0"
			aria-hidden="true"
			><rect x="4" y="4" width="26" height="92" fill="currentColor" /><path
				d="M30 4A66 66 0 0 1 96 70H30Z"
				fill="var(--sh-accent)"
			/></svg
		>{/if}repose</span
>
