<script lang="ts">
	// The wordmark: the name in the heading face, nothing else.
	let { size = 'base' }: { size?: 'sm' | 'base' } = $props();
</script>

<span class="font-display font-semibold tracking-tight {size === 'sm' ? 'text-base' : 'text-lg'}"
	>repose</span
>
