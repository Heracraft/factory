<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { handleSignInCallback } from '$lib/auth.svelte';

	onMount(async () => {
		try {
			await handleSignInCallback(location.href);
			await goto('/projects');
		} catch {
			toast.error('Sign-in was cancelled or failed; try again.');
			await goto('/');
		}
	});
</script>

<svelte:head>
	<title>Signing in… — repose</title>
</svelte:head>

<p class="p-16 text-center text-zinc-500 dark:text-zinc-400">Signing in…</p>
