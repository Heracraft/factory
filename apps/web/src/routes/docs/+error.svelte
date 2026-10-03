<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
</script>

<svelte:head><title>Not found · repose docs</title></svelte:head>

<main id="main" class="pt-8 pb-24">
	<h1 class="text-3xl font-semibold">No such page</h1>
	<p class="mt-3 text-ink-muted">
		{#if page.status === 404 && page.params.slug}
			There's no docs page called “{page.params.slug}”.
		{:else}
			This page could not be shown.
		{/if}
		<a href={resolve('/docs')} class="link">The overview</a> lists everything.
	</p>
</main>
