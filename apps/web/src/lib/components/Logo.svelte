<script lang="ts">
	// The wordmark: the name in the heading face. The landing puts the mark,
	// the quartered ring from its shape set, in front of it.
	import Shape from './landing/Shape.svelte';

	let { size = 'base', mark = false }: { size?: 'sm' | 'base'; mark?: boolean } = $props();
</script>

<span
	class="inline-flex items-center gap-2 font-display font-semibold tracking-tight {size === 'sm'
		? 'text-base'
		: 'text-lg'}"
	>{#if mark}<span class="{size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'} shrink-0"
			><Shape kind="ring" /></span
		>{/if}repose</span
>
