<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { env } from '$env/dynamic/public';
	import { toast } from 'svelte-sonner';
	import { getMe, billingSetup, billingPortal, billingInvoices, getUsage } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { money, dateTime } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import UsageChart from '$lib/components/UsageChart.svelte';
	import type { Me } from '$lib/api/types';
	import type { Stripe, StripeElements } from '@stripe/stripe-js';

	let me = $state<Me | undefined>(undefined);
	let billingDisabled = $state(false);
	let invoices = $state<
		Array<{ id: string; created_at: string; amount_cents: number; status: string; pdf_url?: string }>
	>([]);
	let usageRows = $state<Array<{ day: string; small: number; large: number; xl: number }>>([]);

	let stripe: Stripe | undefined;
	let elements: StripeElements | undefined;
	let cardContainer: HTMLDivElement | undefined;
	let savingCard = $state(false);
	let portalBusy = $state(false);

	async function load() {
		try {
			me = await getMe();
		} catch (err) {
			toastApiError(err, 'Could not load billing status.');
		}

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

	onMount(load);
	onDestroy(() => elements?.getElement('payment')?.destroy());

	async function setupCard() {
		if (!env.PUBLIC_STRIPE_PUBLISHABLE_KEY) {
			toast.error('Stripe is not configured in this environment.');
			return;
		}
		try {
			const { client_secret } = await billingSetup();
			const { loadStripe } = await import('@stripe/stripe-js');
			stripe = (await loadStripe(env.PUBLIC_STRIPE_PUBLISHABLE_KEY)) ?? undefined;
			if (!stripe || !cardContainer) return;
			elements = stripe.elements({ clientSecret: client_secret });
			elements.create('payment').mount(cardContainer);
		} catch (err) {
			if (err instanceof ApiError && err.code === 'billing_disabled') billingDisabled = true;
			else toastApiError(err, 'Could not start card setup.');
		}
	}

	async function confirmCard() {
		if (!stripe || !elements) return;
		savingCard = true;
		const { error } = await stripe.confirmSetup({
			elements,
			confirmParams: { return_url: `${location.origin}/billing` },
			redirect: 'if_required'
		});
		savingCard = false;
		if (error) {
			toast.error(error.message ?? 'Could not save the card.');
			return;
		}
		toast.success('Card saved.');
		await load();
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
				{money(me.billing.trial_credit_cents)} trial credit remaining.
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
			<h2 class="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Card on file</h2>
			{#if me && !me.billing.has_card && !elements}
				<button type="button" class="btn mt-3" onclick={setupCard}>Add a card</button>
			{/if}
			<div bind:this={cardContainer} class="mt-3"></div>
			{#if elements}
				<button type="button" class="btn mt-3" disabled={savingCard} onclick={confirmCard}>
					{savingCard ? 'Saving…' : 'Save card'}
				</button>
			{/if}
			<div class="mt-3">
				<button type="button" class="btn-ghost px-0" disabled={portalBusy} onclick={openPortal}>
					Manage in Stripe →
				</button>
			</div>
		</div>

		<div class="form-section">
			<h2 class="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Invoices</h2>
			{#if invoices.length === 0}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">No invoices yet.</p>
			{:else}
				<ul class="mt-2">
					{#each invoices as inv (inv.id)}
						<li class="row flex items-center justify-between text-sm">
							<span>{dateTime(inv.created_at)} · <span class="badge">{inv.status}</span></span>
							<span>
								{money(inv.amount_cents)}
								{#if inv.pdf_url}<a href={inv.pdf_url} class="link ml-2">PDF</a>{/if}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}

	<div class="form-section">
		<h2 class="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Usage this month</h2>
		{#if usageRows.length === 0}
			<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">No usage yet this month.</p>
		{:else}
			<div class="mt-3">
				<UsageChart rows={usageRows} />
			</div>
		{/if}
	</div>
</PageShell>
