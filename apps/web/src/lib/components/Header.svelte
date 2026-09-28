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
	<div class="mx-auto flex h-14 max-w-5xl items-center justify-between gap-4 px-4 sm:gap-6 sm:px-5">
		<a class="shrink-0" href={resolve('/projects')} aria-label="repose, projects"><Logo /></a>
		<!-- At phone width Sign out moves to the Account page, and on a very
		     narrow screen the links scroll sideways inside the header rather
		     than pushing the page wider. -->
		<nav
			class="flex h-full min-w-0 items-stretch gap-3 overflow-x-auto text-sm [scrollbar-width:none] sm:gap-6"
			aria-label="Main"
		>
			{#each links as link (link.href)}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- link.href is built with resolve() in the links array above -->
				<a
					href={link.href}
					aria-current={isCurrent(link.href) ? 'page' : undefined}
					class="-mb-px flex shrink-0 items-center border-b {isCurrent(link.href)
						? 'border-zinc-900 text-zinc-950 dark:border-zinc-100 dark:text-zinc-50'
						: 'border-transparent text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'}"
					>{link.label}</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/each}
			<button
				type="button"
				class="hidden shrink-0 cursor-pointer whitespace-nowrap sm:block text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
				onclick={() => signOut()}>Sign out</button
			>
		</nav>
	</div>
</header>
