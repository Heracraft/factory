<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { signOut } from '$lib/auth.svelte';

	const links = [
		{ href: resolve('/projects'), label: 'Projects' },
		{ href: resolve('/billing'), label: 'Billing' },
		{ href: resolve('/settings'), label: 'Settings' },
		{ href: resolve('/account'), label: 'Account' }
	];

	function isCurrent(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
	}

	function linkClass(href: string): string {
		return isCurrent(href)
			? 'text-blue-700 dark:text-blue-400'
			: 'text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100';
	}
</script>

<header class="border-b border-zinc-200 dark:border-zinc-800">
	<div
		class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-x-6 gap-y-2 px-5 py-4"
	>
		<a href={resolve('/projects')} class="font-display text-lg font-semibold">repose</a>
		<nav class="flex flex-wrap items-center gap-5 text-sm">
			{#each links as link (link.href)}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- link.href is built with resolve() in the links array above -->
				<a href={link.href} class={linkClass(link.href)}>{link.label}</a>
			{/each}
			<button type="button" class="btn-ghost" onclick={() => signOut()}>Sign out</button>
		</nav>
	</div>
</header>
