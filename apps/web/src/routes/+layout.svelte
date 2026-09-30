<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { Toaster } from 'svelte-sonner';
	import { authState, initAuth } from '$lib/auth.svelte';
	import { reachability } from '$lib/api/reachability.svelte';
	import Header from '$lib/components/Header.svelte';

	let { children } = $props();

	// Routes reachable while signed out: these, and everything under /docs.
	const PUBLIC_PATHS = new Set(['/', '/callback', '/terms', '/privacy', '/refunds']);
	function isPublic(path: string): boolean {
		return PUBLIC_PATHS.has(path) || path === '/docs' || path.startsWith('/docs/');
	}

	onMount(() => {
		void initAuth();
	});

	// Signed-out visitors on a private route go to the landing page. A
	// signed-in visitor may read the landing page too (DECISIONS I-330); its
	// header offers the dashboard instead of sign-in, and signing in itself
	// still lands on /projects (routes/callback).
	$effect(() => {
		if (authState.authenticated === undefined) return;
		if (!authState.authenticated && !isPublic(page.url.pathname)) {
			void goto(resolve('/'));
		}
	});

	let showHeader = $derived(authState.authenticated === true && !isPublic(page.url.pathname));
	let showChildren = $derived(authState.authenticated === true || isPublic(page.url.pathname));
</script>

<!-- No richColors: layout.css gives each toast type its banner's colours. -->
<Toaster theme="system" position="bottom-right" />

{#if !reachability.ok}
	<div
		class="border-b border-red-600/40 bg-red-50 px-4 py-2 text-center text-sm font-medium text-red-800 dark:border-red-400/30 dark:bg-red-950 dark:text-red-200"
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
