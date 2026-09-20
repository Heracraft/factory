<script lang="ts">
	import raw from '../../content/legal/terms.md?raw';
	import LegalPage from '$lib/components/LegalPage.svelte';
</script>

<svelte:head>
	<title>Terms — repose</title>
</svelte:head>

<LegalPage {raw} />
