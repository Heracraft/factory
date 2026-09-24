<script lang="ts">
	import { docBySlug } from '$lib/docs';
	import DocPage from '$lib/components/DocPage.svelte';

	const doc = docBySlug('index')!;
</script>

<DocPage {doc} />
