<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { docBySlug, movedDoc } from '$lib/docs';
	import DocPage from '$lib/components/DocPage.svelte';

	let doc = $derived(docBySlug(page.params.slug ?? ''));
	let moved = $derived(movedDoc(page.params.slug ?? ''));

	$effect(() => {
		if (moved)
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve()d, plus the old link's #section
			void goto(resolve('/docs/[slug]', { slug: moved }) + page.url.hash, { replaceState: true });
	});
</script>

<svelte:head>
	{#if !doc}<title>Not found · repose docs</title>{/if}
</svelte:head>

{#if doc}
	{#key doc.slug}
		<DocPage {doc} />
	{/key}
{:else if !moved}
	<main class="pt-8 pb-24">
		<h1 class="text-3xl font-semibold">No such page</h1>
		<p class="mt-3 text-zinc-600 dark:text-zinc-400">
			There's no docs page called “{page.params.slug}”.
			<a href={resolve('/docs')} class="link">The overview</a> lists everything.
		</p>
	</main>
{/if}
