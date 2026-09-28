<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { authState, signIn } from '$lib/auth.svelte';
	import { docsBySection, search } from '$lib/docs';
	import Logo from '$lib/components/Logo.svelte';

	let { children } = $props();

	const groups = docsBySection();
	let query = $state('');
	let hits = $derived(search(query));
	let menuOpen = $state(false);
	let nav: HTMLElement | undefined = $state();
	let menuButton: HTMLButtonElement | undefined = $state();

	let current = $derived(page.params.slug ?? 'index');

	function href(slug: string): string {
		return slug === 'index' ? resolve('/docs') : resolve('/docs/[slug]', { slug });
	}

	// The sidebar stays mounted at every width (DECISIONS I-344): below lg it
	// is a drawer moved off screen, never display:none, so its scroll and the
	// search keep their state across opening, closing and navigating. The
	// scroll also survives a reload, per tab. Only closing animates
	// visibility: opening shows the drawer at once, so it can take focus.
	const SCROLL_KEY = 'repose.docs.nav.scroll';

	/** Scrolls the sidebar so the current page's link shows, if it doesn't. */
	function revealCurrent() {
		const link = nav?.querySelector<HTMLElement>('[aria-current="page"]');
		if (!nav || !link) return;
		const box = nav.getBoundingClientRect();
		const at = link.getBoundingClientRect();
		if (at.top < box.top || at.bottom > box.bottom) {
			nav.scrollTop += at.top - box.top - nav.clientHeight / 3;
		}
	}

	onMount(() => {
		try {
			const saved = Number(sessionStorage.getItem(SCROLL_KEY));
			if (nav && saved > 0) nav.scrollTop = saved;
		} catch {
			// Storage blocked: the sidebar starts at the top.
		}
		revealCurrent();
	});

	let saving = false;
	function onNavScroll() {
		if (saving) return;
		saving = true;
		requestAnimationFrame(() => {
			saving = false;
			try {
				if (nav) sessionStorage.setItem(SCROLL_KEY, String(Math.round(nav.scrollTop)));
			} catch {
				// Storage blocked: nothing to keep.
			}
		});
	}

	// A navigation, a search hit's #heading on this page included, closes the
	// drawer, and the sidebar follows to the page it lands on.
	$effect(() => {
		void page.url.href;
		menuOpen = false;
		void tick().then(revealCurrent);
	});

	async function openMenu() {
		menuOpen = true;
		await tick();
		revealCurrent();
		nav?.focus({ preventScroll: true });
	}

	function closeMenu(returnFocus = true) {
		if (!menuOpen) return;
		menuOpen = false;
		if (returnFocus) menuButton?.focus();
	}

	// The page behind an open drawer doesn't scroll.
	$effect(() => {
		if (!menuOpen) return;
		const root = document.documentElement;
		const before = root.style.overflow;
		root.style.overflow = 'hidden';
		return () => {
			root.style.overflow = before;
		};
	});
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && menuOpen) closeMenu();
	}}
/>

<!-- eslint-disable svelte/no-navigation-without-resolve -- every internal href here comes from href(), which builds it with resolve() -->

<header class="sticky top-0 z-30 border-b border-[var(--rule)] bg-[var(--page)]">
	<div class="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-5">
		<div class="flex items-center gap-3">
			<button
				bind:this={menuButton}
				type="button"
				class="-ml-2 rounded-sm p-2 text-zinc-700 hover:bg-[var(--sunken)] hover:text-zinc-950 lg:hidden dark:text-zinc-300 dark:hover:text-zinc-50"
				aria-label={menuOpen ? 'Close the docs menu' : 'Open the docs menu'}
				aria-expanded={menuOpen}
				aria-controls="docs-nav"
				onclick={() => (menuOpen ? closeMenu() : openMenu())}
			>
				<svg viewBox="0 0 20 20" class="h-5 w-5" aria-hidden="true" fill="none">
					{#if menuOpen}
						<path d="M5 5l10 10M15 5L5 15" stroke="currentColor" stroke-width="1.5" />
					{:else}
						<path d="M3 5.5h14M3 10h14M3 14.5h14" stroke="currentColor" stroke-width="1.5" />
					{/if}
				</svg>
			</button>
			<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
			<a
				href={resolve('/docs')}
				class="text-sm text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
				>Docs</a
			>
		</div>
		<nav class="flex items-center gap-5 text-sm" aria-label="Site">
			<a
				href="https://github.com/Heracraft/repose"
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>GitHub</a
			>
			{#if authState.authenticated}
				<a href={resolve('/projects')} class="btn-quiet !py-1.5">Dashboard</a>
			{:else}
				<button type="button" class="btn-quiet !py-1.5" onclick={() => signIn()}>Sign in</button>
			{/if}
		</nav>
	</div>
</header>

{#if menuOpen}
	<!-- The backdrop is a pointer target only; Escape and the menu button close the drawer from the keyboard. -->
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="fixed inset-x-0 top-14 bottom-0 z-10 bg-zinc-950/25 lg:hidden dark:bg-black/50"
		onclick={() => closeMenu()}
	></div>
{/if}

<div class="mx-auto flex max-w-6xl gap-10 px-5">
	<aside
		id="docs-nav"
		bind:this={nav}
		onscroll={onNavScroll}
		tabindex="-1"
		aria-label="Docs menu"
		class="fixed top-14 bottom-0 left-0 z-20 w-[min(20rem,85vw)] overflow-y-auto overscroll-contain border-r border-[var(--rule)] bg-[var(--page)] px-5 pb-10 duration-200 ease-out outline-none motion-reduce:transition-none {menuOpen
			? 'visible translate-x-0 transition-[translate]'
			: 'invisible -translate-x-full transition-[translate,visibility]'} lg:visible lg:sticky lg:top-14 lg:bottom-auto lg:z-auto lg:h-[calc(100dvh-3.5rem)] lg:w-60 lg:shrink-0 lg:translate-x-0 lg:border-r-0 lg:bg-transparent lg:px-0 lg:transition-none"
	>
		<div class="pt-6">
			<label for="docs-search" class="sr-only">Search the docs</label>
			<input
				id="docs-search"
				type="search"
				class="field w-full"
				placeholder="Search the docs"
				autocomplete="off"
				bind:value={query}
			/>
		</div>

		{#if query.trim()}
			<ul class="mt-4 space-y-1" aria-label="Search results">
				{#each hits as hit (hit.doc.slug)}
					<li>
						<a
							href={href(hit.doc.slug) + (hit.heading ? `#${hit.heading.id}` : '')}
							class="block rounded-sm px-2 py-1.5 hover:bg-[var(--sunken)]"
						>
							<span class="block text-sm font-medium">
								{hit.doc.title}{#if hit.heading}<span class="text-zinc-600 dark:text-zinc-400">
										› {hit.heading.text}</span
									>{/if}
							</span>
							<span class="mt-0.5 block text-xs text-zinc-600 dark:text-zinc-400"
								>{hit.snippet}</span
							>
						</a>
					</li>
				{:else}
					<li class="px-2 py-1.5 text-sm text-zinc-600 dark:text-zinc-400">Nothing matches.</li>
				{/each}
			</ul>
		{:else}
			<nav class="mt-5" aria-label="Docs">
				{#each groups as group (group.section)}
					<p class="mt-5 mb-1 px-2 text-sm font-medium text-zinc-950 first:mt-0 dark:text-zinc-50">
						{group.section}
					</p>
					<ul>
						{#each group.docs as doc (doc.slug)}
							<li>
								<a
									href={href(doc.slug)}
									aria-current={current === doc.slug ? 'page' : undefined}
									class="block rounded-sm px-2 py-1 text-sm {current === doc.slug
										? 'bg-[var(--sunken)] font-medium text-zinc-950 dark:text-zinc-50'
										: 'text-zinc-700 hover:text-zinc-950 dark:text-zinc-300 dark:hover:text-zinc-50'}"
									>{doc.title}</a
								>
							</li>
						{/each}
					</ul>
				{/each}
			</nav>
		{/if}
	</aside>

	<div class="min-w-0 flex-1">
		{@render children()}
	</div>
</div>
