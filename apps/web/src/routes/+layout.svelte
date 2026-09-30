<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { afterNavigate, goto } from '$app/navigation';
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

	// The skip link is drawn once the page runs, not in the prerendered
	// HTML: the prerender fails on a #main link from a page whose <main>
	// has no id (the landing's, in routes/+page.svelte).
	let mounted = $state(false);
	onMount(() => {
		mounted = true;
		void initAuth();
	});

	// The skip link moves focus to whichever page's <main> is on screen.
	// The dashboard's, the docs' and the legal pages' carry id="main"; the
	// landing's has none yet (routes/+page.svelte belongs to the
	// landing-critique branch), so it is given the id after each
	// navigation. Without a target the link is broken, and Lighthouse's
	// skip-link audit fails the landing.
	afterNavigate(() => {
		const main = document.querySelector('main');
		if (main && !main.id) main.id = 'main';
	});
	function skipToMain(e: MouseEvent) {
		const main = document.querySelector('main');
		if (!main) return;
		e.preventDefault();
		main.tabIndex = -1;
		main.focus();
	}

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

<!-- The first thing a keyboard reaches on every page: past the header's
     links to the page's own content (WCAG 2.4.1). Hidden until focused. -->
{#if mounted}
	<a href="#main" class="skip-link" onclick={skipToMain}>Skip to content</a>
{/if}

<!-- No richColors: layout.css gives each toast type its banner's colours.
     From 600px up the toast's right edge is the content column's (the
     header's max-w-5xl with its 20px gutter), not the window's, so at 1440
     it lines up under the header's last link instead of 200px past it. On a
     phone it spans the width between the page's 20px gutters (sonner's
     own 16px missed the column by 4px on each side), at the bottom, where
     it can be swiped away (DECISIONS I-391 keeps it off the header). -->
<Toaster
	theme="system"
	position="bottom-right"
	offset={{ right: 'max(24px, calc((100% - 64rem) / 2 + 1.25rem))', bottom: '24px' }}
	mobileOffset={{ left: '20px', right: '20px', top: '12px', bottom: '20px' }}
/>

<!-- The live region is always in the page and only its contents change:
     a role=status inserted with its text already in it is often read by
     no screen reader (I-393). It appears after the page has loaded, and a
     screen reader should hear it without losing its place. Empty, it
     draws nothing. -->
<div id="outage" role="status">
	{#if !reachability.ok}
		<!-- The .banner--error colours, as a strip across the top: square,
		     no side edges and no margin, since it is the page's edge and
		     not a box in the column. -->
		<div
			class="banner banner--error mb-0 rounded-none border-x-0 border-t-0 py-2 text-center font-medium"
		>
			{reachability.reason === 'network'
				? 'Cannot reach the API. Retrying…'
				: 'The API is failing right now. Retrying…'}
		</div>
	{/if}
</div>

{#if showHeader}
	<Header />
{/if}

{#if showChildren}
	{@render children()}
{/if}
