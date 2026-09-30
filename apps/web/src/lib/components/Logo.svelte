<script lang="ts">
	// The wordmark: the name in the heading face, behind the mark. The mark
	// is the owner's notebook sketch, traced (DECISIONS I-363): a thin cross
	// in the text's ink, its crossing left of centre and low, and three
	// blocks hugging the crossing in the landing's palette: a skinny one in
	// grey above-left, a middle square in the light grey above-right on the
	// arm, and the big square in the one blue below-right. Flat, no stroke;
	// the favicon is this same drawing.
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
			><rect x="36" y="18" width="7" height="32" fill="var(--sh-grey)" /><rect
				x="47"
				y="36"
				width="12"
				height="14"
				fill="var(--sh-light)"
			/><rect x="47" y="54" width="20" height="18" fill="var(--sh-accent)" /><rect
				x="43"
				y="4"
				width="4"
				height="92"
				fill="currentColor"
			/><rect x="13" y="50" width="74" height="4" fill="currentColor" /></svg
		>{/if}repose</span
>
