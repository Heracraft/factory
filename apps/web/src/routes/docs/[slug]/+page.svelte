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

{#if doc}
	{#key doc.slug}
		<DocPage {doc} />
	{/key}
{:else if moved}
	<!-- An old slug's page holds the target's link, not nothing: the
	     prerendered HTML is what a reader with scripts off gets, and the
	     layout's skip link (#main) needs a target on every page. -->
	<main id="main" class="pt-8 pb-24">
		<p class="text-ink-muted">
			This page moved to <a href={resolve('/docs/[slug]', { slug: moved })} class="link"
				>{docBySlug(moved)?.title ?? moved}</a
			>.
		</p>
	</main>
{/if}
