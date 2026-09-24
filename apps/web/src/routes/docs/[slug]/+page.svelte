<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { docBySlug } from '$lib/docs';
	import DocPage from '$lib/components/DocPage.svelte';

	let doc = $derived(docBySlug(page.params.slug ?? ''));
</script>

<svelte:head>
	{#if !doc}<title>Not found · repose docs</title>{/if}
</svelte:head>

{#if doc}
	{#key doc.slug}
		<DocPage {doc} />
	{/key}
{:else}
	<main class="pt-8 pb-24">
		<h1 class="text-3xl font-semibold">No such page</h1>
		<p class="mt-3 text-zinc-600 dark:text-zinc-400">
			There's no docs page called “{page.params.slug}”.
			<a href={resolve('/docs')} class="link">The overview</a> lists everything.
		</p>
	</main>
{/if}
