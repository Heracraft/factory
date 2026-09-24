<script lang="ts">
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

	let current = $derived(page.params.slug ?? 'index');

	function href(slug: string): string {
		return slug === 'index' ? resolve('/docs') : resolve('/docs/[slug]', { slug });
	}

	// Close the phone menu and clear the search on every navigation.
	$effect(() => {
		void page.url.pathname;
		menuOpen = false;
		query = '';
	});
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- every internal href here comes from href(), which builds it with resolve() -->

<header class="sticky top-0 z-20 border-b border-[var(--rule)] bg-[var(--page)]">
	<div class="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-5">
		<div class="flex items-center gap-3">
			<a href={resolve('/')} aria-label="repose, home"><Logo /></a>
			<a
				href={resolve('/docs')}
				class="text-sm text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
				>Docs</a
			>
		</div>
		<nav class="flex items-center gap-5 text-sm" aria-label="Site">
			<a
				href="https://github.com/Heracraft/factory"
				class="hidden text-zinc-600 hover:text-zinc-900 sm:inline dark:text-zinc-400 dark:hover:text-zinc-100"
				>GitHub</a
			>
			{#if authState.authenticated}
				<a href={resolve('/projects')} class="btn-quiet !py-1.5">Dashboard</a>
			{:else}
				<button type="button" class="btn-quiet !py-1.5" onclick={() => signIn()}>Sign in</button>
			{/if}
			<button
				type="button"
				class="btn-quiet !py-1.5 lg:hidden"
				aria-expanded={menuOpen}
				aria-controls="docs-nav"
				onclick={() => (menuOpen = !menuOpen)}>{menuOpen ? 'Close' : 'Menu'}</button
			>
		</nav>
	</div>
</header>

<div class="mx-auto flex max-w-6xl gap-10 px-5">
	<aside
		id="docs-nav"
		class="{menuOpen
			? 'fixed inset-x-0 top-14 bottom-0 z-10 block overflow-y-auto bg-[var(--page)] px-5 pb-10'
			: 'hidden'} lg:sticky lg:top-14 lg:block lg:h-[calc(100dvh-3.5rem)] lg:w-60 lg:shrink-0 lg:overflow-y-auto lg:px-0 lg:pb-10"
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
					<p
						class="mt-5 mb-1.5 px-2 text-xs font-medium tracking-wide text-zinc-600 uppercase first:mt-0 dark:text-zinc-400"
					>
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
