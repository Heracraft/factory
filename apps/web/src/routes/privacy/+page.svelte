<script lang="ts">
	import raw from '../../content/legal/privacy.md?raw';
	import LegalPage from '$lib/components/LegalPage.svelte';
</script>

<svelte:head>
	<title>Privacy — repose</title>
</svelte:head>

<LegalPage {raw} />
