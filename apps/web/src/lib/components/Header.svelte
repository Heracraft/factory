<script lang="ts">
	import { page } from '$app/state';
	import { signOut } from '$lib/auth.svelte';

	const links = [
		{ href: '/projects', label: 'Projects' },
		{ href: '/billing', label: 'Billing' },
		{ href: '/settings', label: 'Settings' },
		{ href: '/account', label: 'Account' }
	];

	function isCurrent(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
	}
</script>

<header class="border-b border-zinc-200 dark:border-zinc-800">
	<div
		class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-x-6 gap-y-2 px-5 py-4"
	>
		<a href="/projects" class="font-display text-lg font-semibold">repose</a>
		<nav class="flex flex-wrap items-center gap-5 text-sm">
			{#each links as link (link.href)}
				<a
					href={link.href}
					class={isCurrent(link.href)
						? 'text-blue-700 dark:text-blue-400'
						: 'text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'}
				>
					{link.label}
				</a>
			{/each}
			<button type="button" class="btn-ghost" onclick={() => signOut()}>Sign out</button>
		</nav>
	</div>
</header>
