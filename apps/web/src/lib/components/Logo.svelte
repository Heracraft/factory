<script lang="ts">
	// The wordmark: the name in the heading face, behind the mark. The mark
	// is the owner's notebook sketch, traced (DECISIONS I-363): a thin cross
	// in the text's ink, its crossing left of centre and low, and three
	// blocks hugging the crossing in the landing's palette: a skinny one in
	// grey above-left, a middle square in the light grey above-right on the
	// arm, and the big square in the one blue below-right. Flat, no stroke;
	// the favicon is this same drawing. The viewBox is cropped to the
	// drawing so the thin stems get every pixel the header gives the mark.
	// word=false draws the mark alone, for a header row on a phone that has
	// no room for both; the link around it carries the name.
	let {
		size = 'base',
		mark = false,
		word = true
	}: { size?: 'sm' | 'base'; mark?: boolean; word?: boolean } = $props();
</script>

<span
	class="inline-flex items-center gap-2 font-display font-semibold tracking-tight {size === 'sm'
		? 'text-base'
		: 'text-lg'}"
	>{#if mark}<svg
			viewBox="13 4 74 92"
			class="{size === 'sm' ? 'h-6' : 'h-7'} w-auto shrink-0"
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
		>{/if}{#if word}repose{/if}</span
>
