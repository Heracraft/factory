<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { authState, signIn } from '$lib/auth.svelte';
	import { docBySlug, docsBySection, search } from '$lib/docs';
	import HeaderFrame from '$lib/components/HeaderFrame.svelte';

	let { children } = $props();

	const groups = docsBySection();
	let query = $state('');
	let hits = $derived(search(query));
	let menuOpen = $state(false);
	let nav: HTMLElement | undefined = $state();
	let menuButton: HTMLButtonElement | undefined = $state();

	let current = $derived(page.params.slug ?? 'index');
	// The open page's sections, listed under its link in the sidebar. They
	// took a right rail before; in the header's max-w-5xl column a rail
	// would leave the text too narrow for the docs' 70-column code.
	let sections = $derived((docBySlug(current)?.headings ?? []).filter((h) => h.depth === 2));

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

<!-- The shared header frame, kept in view while you read. The menu button
     sits at the right end so the logo is at the same x as on the dashboard
     and the legal pages; the drawer it opens still comes from the left, where
     the sidebar lives from lg up. -->
<HeaderFrame home={resolve('/')} label="repose, home" sticky>
	{#snippet lead()}
		<a href={resolve('/docs')} class="text-sm text-ink-muted hover:text-ink">Docs</a>
	{/snippet}
	<nav class="flex items-center gap-4 text-sm sm:gap-5" aria-label="Site">
		<a
			href="https://github.com/Heracraft/repose"
			class="hidden text-ink-muted hover:text-ink sm:inline">GitHub</a
		>
		{#if authState.authenticated}
			<a href={resolve('/projects')} class="btn-quiet btn--sm">Dashboard</a>
		{:else}
			<button type="button" class="btn-quiet btn--sm" onclick={() => signIn()}>Sign in</button>
		{/if}
		<button
			bind:this={menuButton}
			type="button"
			class="-mr-2 rounded-sm p-2 text-ink-muted hover:bg-sunken hover:text-ink lg:hidden"
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
	</nav>
</HeaderFrame>

{#if menuOpen}
	<!-- The backdrop is a pointer target only; Escape and the menu button close the drawer from the keyboard. -->
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="fixed inset-x-0 top-14 bottom-0 z-10 bg-zinc-950/25 lg:hidden dark:bg-black/50"
		onclick={() => closeMenu()}
	></div>
{/if}

<div class="mx-auto flex max-w-5xl gap-10 px-5">
	<aside
		id="docs-nav"
		bind:this={nav}
		onscroll={onNavScroll}
		tabindex="-1"
		aria-label="Docs menu"
		class="fixed top-14 bottom-0 left-0 z-20 w-[min(20rem,85vw)] overflow-y-auto overscroll-contain border-r border-rule bg-page px-5 pb-10 duration-200 ease-out outline-none motion-reduce:transition-none {menuOpen
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
							class="block rounded-sm px-2 py-1.5 hover:bg-sunken"
						>
							<span class="block text-sm font-medium">
								{hit.doc.title}{#if hit.heading}<span class="text-ink-muted">
										› {hit.heading.text}</span
									>{/if}
							</span>
							<span class="mt-0.5 block text-xs text-ink-muted">{hit.snippet}</span>
						</a>
					</li>
				{:else}
					<li class="px-2 py-1.5 text-sm text-ink-muted">Nothing matches.</li>
				{/each}
			</ul>
		{:else}
			<nav class="mt-5" aria-label="Docs">
				{#each groups as group (group.section)}
					<p class="mt-5 mb-1 px-2 text-sm font-medium text-ink first:mt-0">
						{group.section}
					</p>
					<ul>
						{#each group.docs as doc (doc.slug)}
							<li>
								<a
									href={href(doc.slug)}
									aria-current={current === doc.slug ? 'page' : undefined}
									class="block rounded-sm px-2 py-1 text-sm {current === doc.slug
										? 'bg-sunken font-medium text-ink'
										: 'text-ink-muted hover:text-ink'}">{doc.title}</a
								>
								{#if current === doc.slug && sections.length > 1}
									<ul class="mt-1 mb-2 ml-2 border-l border-rule pl-3" aria-label="On this page">
										{#each sections as h (h.id)}
											<li>
												<a
													href={`#${h.id}`}
													class="block py-1 text-compact leading-5 text-ink-muted hover:text-ink"
													>{h.text}</a
												>
											</li>
										{/each}
									</ul>
								{/if}
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
