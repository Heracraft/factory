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

<!-- No richColors: layout.css gives each toast type its banner's colours.
     From 600px up the toast's right edge is the content column's (the
     header's max-w-5xl with its 20px gutter), not the window's, so at 1440
     it lines up under the header's last link instead of 200px past it. On a
     phone sonner spans the width at the bottom; a toast there can be swiped
     away. -->
<Toaster
	theme="system"
	position="bottom-right"
	offset={{ right: 'max(24px, calc((100% - 64rem) / 2 + 1.25rem))', bottom: '24px' }}
/>

{#if !reachability.ok}
	<!-- The .banner--error colours, as a strip across the top: square, no
	     side edges and no margin, since it is the page's edge and not a box
	     in the column. role=status: it appears after the page has loaded, and
	     a screen reader should hear it without losing its place. -->
	<div
		class="banner banner--error mb-0 rounded-none border-x-0 border-t-0 py-2 text-center font-medium"
		role="status"
	>
		{reachability.reason === 'network'
			? 'Cannot reach the API. Retrying…'
			: 'The API is failing right now. Retrying…'}
	</div>
{/if}

{#if showHeader}
	<Header />
{/if}

{#if showChildren}
	{@render children()}
{/if}
