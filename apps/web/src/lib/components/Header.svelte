<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { signOut } from '$lib/auth.svelte';
	import Logo from './Logo.svelte';

	const links = [
		{ href: resolve('/projects'), label: 'Projects' },
		{ href: resolve('/billing'), label: 'Billing' },
		{ href: resolve('/settings'), label: 'Settings' },
		{ href: resolve('/account'), label: 'Account' }
	];

	function isCurrent(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
	}
</script>

<header class="border-b border-[var(--rule)]">
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between gap-6 px-5">
		<a href={resolve('/projects')} aria-label="repose, projects"><Logo /></a>
		<nav class="flex h-full items-stretch gap-6 text-sm" aria-label="Main">
			{#each links as link (link.href)}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- link.href is built with resolve() in the links array above -->
				<a
					href={link.href}
					aria-current={isCurrent(link.href) ? 'page' : undefined}
					class="-mb-px flex items-center border-b {isCurrent(link.href)
						? 'border-zinc-900 text-zinc-950 dark:border-zinc-100 dark:text-zinc-50'
						: 'border-transparent text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'}"
					>{link.label}</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/each}
			<button
				type="button"
				class="cursor-pointer text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
				onclick={() => signOut()}>Sign out</button
			>
		</nav>
	</div>
</header>
