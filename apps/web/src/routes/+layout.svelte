<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Toaster } from 'svelte-sonner';
	import { authState, initAuth } from '$lib/auth.svelte';
	import { reachability } from '$lib/api/reachability.svelte';
	import Header from '$lib/components/Header.svelte';

	let { children } = $props();

	// Routes reachable while signed out.
	const PUBLIC_PATHS = new Set(['/', '/callback', '/terms', '/privacy']);

	onMount(() => {
		void initAuth();
	});

	$effect(() => {
		if (authState.authenticated === undefined) return;
		const path = page.url.pathname;
		if (!authState.authenticated && !PUBLIC_PATHS.has(path)) {
			void goto('/');
		} else if (authState.authenticated && path === '/') {
			void goto('/projects');
		}
	});

	let showHeader = $derived(authState.authenticated === true && !PUBLIC_PATHS.has(page.url.pathname));
	let showChildren = $derived(
		authState.authenticated === true || PUBLIC_PATHS.has(page.url.pathname)
	);
</script>

<Toaster theme="system" position="bottom-right" richColors />

{#if !reachability.ok}
	<div
		class="border-b border-red-200 bg-red-100 px-4 py-2 text-center text-sm font-medium text-red-700 dark:border-red-500/20 dark:bg-red-500/15 dark:text-red-400"
	>
		Cannot reach the API. Retrying…
	</div>
{/if}

{#if showHeader}
	<Header />
{/if}

{#if showChildren}
	{@render children()}
{/if}
