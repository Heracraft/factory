<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { handleSignInCallback } from '$lib/auth.svelte';

	onMount(async () => {
		try {
			await handleSignInCallback(location.href);
			await goto(resolve('/projects'));
		} catch {
			toast.error('Sign-in was cancelled or failed; try again.');
			await goto(resolve('/'));
		}
	});
</script>

<svelte:head>
	<title>Signing in… — repose</title>
</svelte:head>

<!-- A <main> like every other page's, so the layout's skip link has its
     target here too. -->
<main id="main">
	<p class="p-16 text-center text-ink-muted" role="status">Signing in…</p>
</main>
