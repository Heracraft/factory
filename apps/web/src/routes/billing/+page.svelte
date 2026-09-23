<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import {
		getMe,
		billingSetupCheckout,
		billingPortal,
		billingInvoices,
		getUsage
	} from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { money, dateTime, trialTimeLeft } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import UsageChart from '$lib/components/UsageChart.svelte';
	import type { Invoice, Me } from '$lib/api/types';

	let me = $state<Me | undefined>(undefined);
	let billingDisabled = $state(false);
	let invoices = $state<Invoice[]>([]);
	let usageRows = $state<Array<{ day: string; small: number; large: number; xl: number }>>([]);

	let cardBusy = $state(false);
	let portalBusy = $state(false);

	async function loadMe() {
		try {
			me = await getMe();
		} catch (err) {
			toastApiError(err, 'Could not load billing status.');
		}
	}

	async function load() {
		await loadMe();

		try {
			invoices = await billingInvoices();
		} catch (err) {
			if (err instanceof ApiError && err.code === 'billing_disabled') billingDisabled = true;
			else toastApiError(err, 'Could not load invoices.');
		}

		try {
			const now = new Date();
			const from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().slice(0, 10);
			const to = now.toISOString().slice(0, 10);
			const rows = await getUsage(from, to);
			// A local scratch value, discarded once usageRows is assigned below —
			// a plain Map, not SvelteMap, since nothing reads it reactively.
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const byDay = new Map<string, { day: string; small: number; large: number; xl: number }>();
			for (const r of rows) {
				const entry = byDay.get(r.day) ?? { day: r.day, small: 0, large: 0, xl: 0 };
				entry.small += r.guest_hours.small ?? 0;
				entry.large += r.guest_hours.large ?? 0;
				entry.xl += r.guest_hours.xl ?? 0;
				byDay.set(r.day, entry);
			}
			usageRows = [...byDay.values()].sort((a, b) => (a.day < b.day ? -1 : 1));
		} catch {
			// The rest of the page still works without the chart.
		}
	}

	// Stripe's hosted page sends the user back with ?card=saved or
	// ?card=cancelled (DECISIONS I-182). The card reaches the account through
	// the setup_intent.succeeded webhook, which can land a few seconds after
	// the redirect, so a saved card is waited for briefly.
	async function returnedFromCheckout() {
		const url = new URL(location.href);
		const card = url.searchParams.get('card');
		if (!card) return;
		url.searchParams.delete('card');
		history.replaceState(history.state, '', url.pathname + url.search + url.hash);
		if (card === 'cancelled') {
			toast('No card was added.');
			return;
		}
		if (card !== 'saved') return;
		toast.success('Card saved.');
		for (let i = 0; i < 10 && me && !me.billing.has_card; i++) {
			await new Promise((r) => setTimeout(r, 1500));
			await loadMe();
		}
	}

	onMount(async () => {
		await load();
		await returnedFromCheckout();
	});

	async function addCard() {
		cardBusy = true;
		try {
			const { url } = await billingSetupCheckout();
			location.href = url;
		} catch (err) {
			cardBusy = false;
			if (err instanceof ApiError && err.code === 'billing_disabled') billingDisabled = true;
			else toastApiError(err, 'Could not start card setup.');
		}
	}

	async function openPortal() {
		portalBusy = true;
		try {
			const { url } = await billingPortal();
			location.href = url;
		} catch (err) {
			portalBusy = false;
			if (err instanceof ApiError && err.code === 'billing_disabled') billingDisabled = true;
			else toastApiError(err, 'Could not open the billing portal.');
		}
	}
</script>

<svelte:head>
	<title>Billing — repose</title>
</svelte:head>

<PageShell title="Billing" width="form">
	{#if me}
		{#if me.billing.status === 'past_due'}
			<div class="banner banner--error">Payment past due. Guests stop after 3 days.</div>
		{:else if me.billing.status === 'suspended'}
			<div class="banner banner--error">Account suspended.</div>
		{:else if me.billing.status === 'trial'}
			<div class="banner banner--ok">
				{trialTimeLeft(me.billing.trial_credit_cents)}
			</div>
		{:else if me.billing.status === 'exempt'}
			<div class="banner banner--ok">This account is billing-exempt.</div>
		{/if}
	{/if}

	{#if billingDisabled}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">
			Billing is not enabled for this account yet.
		</p>
	{:else}
		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Card on file</h2>
			{#if me && !me.billing.has_card}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
					A card is needed before a guest can start. Stripe keeps it; repose never sees the number.
				</p>
				<button type="button" class="btn mt-3" disabled={cardBusy} onclick={addCard}>
					{cardBusy ? 'Opening Stripe…' : 'Add a card'}
				</button>
			{:else if me}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">A card is on file.</p>
			{/if}
			<div class="mt-3">
				<button type="button" class="btn-ghost px-0" disabled={portalBusy} onclick={openPortal}>
					Manage card, address and invoices in Stripe →
				</button>
			</div>
		</div>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Invoices</h2>
			{#if invoices.length === 0}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">No invoices yet.</p>
			{:else}
				<ul class="mt-2" aria-label="Invoices">
					{#each invoices as inv (inv.id)}
						<li class="row flex items-center justify-between text-sm">
							<span>
								{dateTime(inv.created_at)}
								{#if inv.number}· {inv.number}{/if}
								· <span class="badge">{inv.status}</span>
							</span>
							<span>
								{money(inv.amount_cents)}
								{#if inv.tax_cents > 0}
									<span class="text-zinc-500 dark:text-zinc-400">(tax {money(inv.tax_cents)})</span>
								{/if}
								{#if inv.hosted_url}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Stripe-hosted URL, not an app route -->
									<a href={inv.hosted_url} class="link ml-2">View</a>
								{/if}
								{#if inv.pdf_url}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Stripe-hosted URL, not an app route -->
									<a href={inv.pdf_url} class="link ml-2">PDF</a>
								{/if}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}

	<div class="form-section">
		<h2 class="font-display text-xl font-semibold">Usage this month</h2>
		{#if usageRows.length === 0}
			<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">No usage yet this month.</p>
		{:else}
			<div class="mt-3">
				<UsageChart rows={usageRows} />
			</div>
		{/if}
	</div>
</PageShell>
