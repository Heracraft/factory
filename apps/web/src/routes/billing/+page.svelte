<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import {
		getMe,
		getBilling,
		billingCheckout,
		billingJoinWaitlist,
		billingChangePlan,
		billingCancel,
		billingResume,
		billingPortal,
		billingInvoices
	} from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { toastApiError } from '$lib/api/toast';
	import { openCheckout, pageTheme } from '$lib/paddle';
	import { money, price, gbs, dateOnly, dateTime, timeUntil } from '$lib/format';
	import PageShell from '$lib/components/PageShell.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import type { Billing, Invoice, Me, Plan, PlanId } from '$lib/api/types';

	let me = $state<Me | undefined>(undefined);
	let billing = $state<Billing | undefined>(undefined);
	let billingDisabled = $state(false);
	let loadFailed = $state(false);
	let invoices = $state<Invoice[]>([]);

	// After Paddle's overlay reports the checkout done (or the user comes
	// back on ?checkout=done), the subscription is still on its way by
	// webhook: "Setting up your plan" polls until it is there (I-289).
	let settingUp = $state(false);
	let setupTimedOut = $state(false);

	let busy = $state<string | undefined>(undefined);
	let changing = $state(false);
	let changeError = $state<string | undefined>(undefined);
	let confirmCancel = $state(false);

	let sub = $derived(billing?.subscription ?? null);
	let plan = $derived<Plan | undefined>(
		billing && sub ? billing.plans.find((p) => p.id === sub.plan) : undefined
	);
	let otherPlan = $derived<Plan | undefined>(
		billing && sub ? billing.plans.find((p) => p.id !== sub.plan) : undefined
	);
	let holdActive = $derived(
		!!billing?.waitlist?.hold_until && new Date(billing.waitlist.hold_until).getTime() > Date.now()
	);
	let anyAvailable = $derived(!!billing?.plans.some((p) => p.available));
	let accountStatus = $derived(me?.billing.status);

	async function load() {
		try {
			me = await getMe();
		} catch (err) {
			toastApiError(err, 'Could not load the account.');
		}
		try {
			billing = await getBilling();
			billingDisabled = false;
			loadFailed = false;
		} catch (err) {
			if (err instanceof ApiError && err.code === 'billing_disabled') billingDisabled = true;
			else {
				loadFailed = true;
				toastApiError(err, 'Could not load billing.');
			}
			return;
		}
		if (billing.subscription) {
			try {
				invoices = await billingInvoices();
			} catch (err) {
				toastApiError(err, 'Could not load invoices.');
			}
		}
	}

	async function waitForSubscription() {
		settingUp = true;
		setupTimedOut = false;
		for (let i = 0; i < 30; i++) {
			await new Promise((r) => setTimeout(r, 2000));
			try {
				billing = await getBilling();
			} catch {
				// A blip while the webhook lands; the next round asks again.
			}
			if (billing?.subscription) {
				settingUp = false;
				void load();
				return;
			}
		}
		settingUp = false;
		setupTimedOut = true;
	}

	function returnedFromCheckout() {
		const url = new URL(location.href);
		if (url.searchParams.get('checkout') !== 'done') return;
		url.searchParams.delete('checkout');
		history.replaceState(history.state, '', url.pathname + url.search + url.hash);
		if (!billing?.subscription) void waitForSubscription();
	}

	onMount(async () => {
		await load();
		returnedFromCheckout();
	});

	async function choose(planId: PlanId) {
		if (!billing) return;
		busy = `choose-${planId}`;
		try {
			const checkout = await billingCheckout(planId);
			await openCheckout({
				transactionId: checkout.transaction_id,
				clientToken: checkout.client_token,
				environment: checkout.environment,
				theme: pageTheme(),
				successUrl: `${location.origin}${resolve('/billing')}?checkout=done`,
				onCompleted: () => void waitForSubscription()
			});
		} catch (err) {
			if (err instanceof ApiError && err.code === 'waitlisted') {
				// The seat went while the page was open: the api put the
				// user on the list, and the page shows that place.
				await load();
			} else if (err instanceof ApiError) {
				toastApiError(err);
			} else {
				toast.error(err instanceof Error ? err.message : 'Could not open the checkout.');
			}
		} finally {
			busy = undefined;
		}
	}

	async function joinWaitlist() {
		busy = 'waitlist';
		try {
			await billingJoinWaitlist();
			await load();
		} catch (err) {
			toastApiError(err, 'Could not join the waitlist.');
		} finally {
			busy = undefined;
		}
	}

	async function changePlan(to: PlanId) {
		busy = 'plan';
		changeError = undefined;
		try {
			await billingChangePlan(to);
			changing = false;
			await load();
		} catch (err) {
			if (err instanceof ApiError && err.code === 'conflict') changeError = err.message;
			else toastApiError(err, 'Could not change the plan.');
		} finally {
			busy = undefined;
		}
	}

	async function cancel() {
		busy = 'cancel';
		try {
			await billingCancel();
			confirmCancel = false;
			await load();
		} catch (err) {
			toastApiError(err, 'Could not cancel the plan.');
		} finally {
			busy = undefined;
		}
	}

	async function resume() {
		busy = 'resume';
		try {
			await billingResume();
			await load();
		} catch (err) {
			toastApiError(err, 'Could not resume the plan.');
		} finally {
			busy = undefined;
		}
	}

	async function openPortal(what?: 'payment_method') {
		busy = what ?? 'portal';
		try {
			const { url } = await billingPortal(what);
			location.href = url;
		} catch (err) {
			busy = undefined;
			toastApiError(err, 'Could not open the billing portal.');
		}
	}

	/** "one large, or two small" for a plan's memory (docs/PRICING.md). */
	function runsAtOnce(p: Plan): string {
		return p.memory_gb >= 16 ? 'one xl, two large, or any mix' : 'one large, or two small';
	}
</script>

<svelte:head>
	<title>Billing — repose</title>
</svelte:head>

{#snippet planCards(plans: Plan[])}
	<ul class="plans" aria-label="Plans">
		{#each plans as p (p.id)}
			<li class="card flex flex-col" data-testid="plan-{p.id}">
				<div class="flex items-baseline justify-between gap-3">
					<h3 class="text-xl font-semibold">{p.name}</h3>
					<p class="text-sm">
						<span class="font-display text-2xl font-semibold">{price(p.price_cents)}</span>
						<span class="text-zinc-500 dark:text-zinc-400"> a month</span>
					</p>
				</div>
				<dl class="mt-4 space-y-1.5 text-sm">
					<div class="flex justify-between gap-4">
						<dt class="text-zinc-500 dark:text-zinc-400">Running at once</dt>
						<dd class="text-right">{p.memory_gb} GB: {runsAtOnce(p)}</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-zinc-500 dark:text-zinc-400">Disk</dt>
						<dd>{p.disk_gb} GB</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-zinc-500 dark:text-zinc-400">Egress a month</dt>
						<dd>{p.egress_gb} GB</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-zinc-500 dark:text-zinc-400">Projects</dt>
						<dd>{p.project_limit}</dd>
					</div>
				</dl>
				<p class="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
					{p.trial_days} days free, card at checkout, cancel any time.
				</p>
				<div class="mt-auto pt-5">
					<button
						type="button"
						class="btn w-full"
						disabled={!!busy || !p.available}
						onclick={() => choose(p.id)}
					>
						{busy === `choose-${p.id}` ? 'Opening checkout…' : `Choose ${p.name}`}
					</button>
					{#if !p.available && billing}
						<p class="mt-2 text-xs text-zinc-500 dark:text-zinc-400">
							Needs {p.seats} seats; {billing.seats.free} free.
						</p>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
{/snippet}

<PageShell title="Billing" width="form">
	{#if billingDisabled}
		<p class="text-sm text-zinc-500 dark:text-zinc-400" data-testid="billing-disabled">
			Billing is not switched on yet.
		</p>
	{:else if loadFailed}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">
			Could not load billing. Try again in a moment.
		</p>
	{:else if !billing}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
	{:else if settingUp}
		<div class="card" data-testid="setting-up" aria-live="polite">
			<h2 class="text-lg font-semibold">Setting up your plan</h2>
			<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
				Your payment went through. The plan appears here in a few seconds.
			</p>
		</div>
	{:else if setupTimedOut && !sub}
		<div class="banner banner--warn">
			The plan is taking longer than usual to arrive. Refresh in a minute; if it is still not here,
			the payment went through and support will sort it out.
		</div>
		{@render planCards(billing.plans)}
	{:else if !sub}
		{#if accountStatus === 'exempt'}
			<div class="banner banner--ok">This account is billing-exempt. No plan is needed.</div>
		{/if}
		{#if holdActive && billing.waitlist?.hold_until}
			<div class="banner banner--ok" data-testid="seat-held">
				Your seat is held until {dateTime(billing.waitlist.hold_until)} ({timeUntil(
					billing.waitlist.hold_until
				)} left). Choose a plan before then.
			</div>
			{@render planCards(billing.plans)}
		{:else if anyAvailable}
			<p class="mb-5 text-sm text-zinc-500 dark:text-zinc-400" data-testid="seats-line">
				{billing.seats.free} of {billing.seats.total} seats left.
			</p>
			{@render planCards(billing.plans)}
		{:else}
			<div class="card" data-testid="full">
				<h2 class="text-lg font-semibold">repose is full</h2>
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
					All {billing.seats.total} seats are taken and {billing.seats.waiting}
					{billing.seats.waiting === 1 ? 'person is' : 'people are'} waiting. A seat frees when a plan
					ends.
				</p>
				{#if billing.waitlist}
					<p class="mt-4 text-sm" data-testid="waitlist-place">
						You're number <b>{billing.waitlist.position}</b> on the waitlist. We'll email
						{me?.email ?? 'you'} when a seat frees; you'll have 72 hours to choose a plan.
					</p>
				{:else}
					<button
						type="button"
						class="btn mt-4"
						disabled={!!busy}
						onclick={joinWaitlist}
						data-testid="join-waitlist"
					>
						{busy === 'waitlist' ? 'Joining…' : 'Join the waitlist'}
					</button>
				{/if}
			</div>
		{/if}
		<p class="mt-8 text-xs text-zinc-500 dark:text-zinc-400">
			Prices in USD before tax; Paddle adds the tax for your country at checkout.
			<a href={resolve('/refunds')} class="link">Refunds</a>.
		</p>
	{:else if plan}
		{#if accountStatus === 'suspended'}
			<div class="banner banner--error" data-testid="status-suspended">
				Your account is suspended: the payment failed three days ago and your machines were stopped.
				Pay the invoice to start them again; snapshots are kept 30 days.
				<button
					type="button"
					class="link mt-2 block"
					disabled={!!busy}
					onclick={() => openPortal('payment_method')}>Update card and pay</button
				>
			</div>
		{:else if sub.status === 'past_due'}
			<div class="banner banner--error" data-testid="status-past-due">
				Your last payment failed. Machines already running keep running; new starts wait until the
				card is updated, and after three days every machine is stopped.
				<button
					type="button"
					class="link mt-2 block"
					disabled={!!busy}
					onclick={() => openPortal('payment_method')}>Update card</button
				>
			</div>
		{/if}

		<section class="card" data-testid="plan">
			<div class="flex flex-wrap items-baseline justify-between gap-3">
				<h2 class="text-xl font-semibold">{plan.name}</h2>
				<p class="text-sm">
					<span class="font-display text-2xl font-semibold">{price(plan.price_cents)}</span>
					<span class="text-zinc-500 dark:text-zinc-400"> a month</span>
				</p>
			</div>
			<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-400" data-testid="plan-status">
				{#if sub.cancel_at}
					Cancelled. Ends {dateOnly(sub.cancel_at)}; machines stop then and snapshots stay 30 days.
				{:else if sub.status === 'trialing' && sub.trial_end}
					Trial. First charge of {price(plan.price_cents)} on {dateOnly(sub.trial_end)}.
				{:else if sub.status === 'past_due'}
					Payment past due since {dateOnly(sub.period_start)}.
				{:else if sub.next_billed_at}
					Active. Renews {dateOnly(sub.next_billed_at)}.
				{:else}
					Active.
				{/if}
				{#if sub.scheduled_plan && otherPlan}
					Changes to {otherPlan.name} on {dateOnly(sub.period_end)}.
				{/if}
			</p>

			<div class="mt-6 space-y-5">
				<Meter
					label="Running now"
					used={billing.usage.running_gb}
					limit={billing.usage.memory_gb}
					format={gbs}
					note={runsAtOnce(plan)}
				/>
				<Meter
					label="Disk allocated"
					used={billing.usage.disk_allocated_gb}
					limit={billing.usage.disk_gb}
					format={gbs}
				/>
				<Meter
					label="Egress this period"
					used={billing.usage.egress_gb}
					limit={billing.usage.egress_included_gb}
					format={gbs}
					note={billing.usage.overage_cents > 0
						? `Over by ${gbs(billing.usage.egress_gb - billing.usage.egress_included_gb)}: ${money(billing.usage.overage_cents)} on the next invoice at $0.05 a GB.`
						: undefined}
				/>
				<p class="text-sm" data-testid="projects-count">
					<span class="font-medium">Projects</span>
					<span class="ml-2 font-mono text-[13px] text-zinc-600 dark:text-zinc-400"
						>{billing.usage.projects} of {billing.usage.project_limit}</span
					>
				</p>
			</div>

			<div
				class="mt-6 flex flex-wrap gap-x-5 gap-y-2 border-t pt-4"
				style="border-color: var(--rule)"
			>
				{#if sub.cancel_at}
					<button type="button" class="btn" disabled={!!busy} onclick={resume}>
						{busy === 'resume' ? 'Resuming…' : 'Resume plan'}
					</button>
				{:else}
					<button
						type="button"
						class="btn-quiet"
						disabled={!!busy}
						onclick={() => {
							changing = !changing;
							changeError = undefined;
						}}>Change plan</button
					>
					<button
						type="button"
						class="btn-ghost-danger"
						disabled={!!busy}
						onclick={() => (confirmCancel = true)}>Cancel plan</button
					>
				{/if}
				<button
					type="button"
					class="btn-ghost ml-auto px-0"
					disabled={!!busy}
					onclick={() => openPortal()}>Manage card and receipts →</button
				>
			</div>

			{#if changing && otherPlan}
				{@const other = otherPlan}
				<div
					class="mt-4 rounded-sm border p-4 text-sm"
					style="border-color: var(--rule)"
					data-testid="change-plan"
				>
					{#if sub.scheduled_plan}
						<p>
							{otherPlan.name} is scheduled for {dateOnly(sub.period_end)}. Keep {plan.name} instead?
						</p>
						<button
							type="button"
							class="btn mt-3"
							disabled={!!busy}
							onclick={() => changePlan(sub.plan)}
						>
							{busy === 'plan' ? 'Saving…' : `Keep ${plan.name}`}
						</button>
					{:else if otherPlan.price_cents > plan.price_cents}
						<p>
							Upgrade to <b>{otherPlan.name}</b> ({price(otherPlan.price_cents)} a month:
							{otherPlan.memory_gb} GB running at once, {otherPlan.disk_gb} GB disk,
							{otherPlan.egress_gb} GB egress). Takes effect at once; Paddle prorates the rest of this
							period.
						</p>
						<button
							type="button"
							class="btn mt-3"
							disabled={!!busy}
							onclick={() => changePlan(other.id)}
						>
							{busy === 'plan' ? 'Upgrading…' : `Upgrade to ${otherPlan.name}`}
						</button>
					{:else}
						<p>
							Downgrade to <b>{otherPlan.name}</b> ({price(otherPlan.price_cents)} a month:
							{otherPlan.memory_gb} GB running at once, {otherPlan.disk_gb} GB disk,
							{otherPlan.egress_gb} GB egress). Takes effect at the renewal on {dateOnly(
								sub.period_end
							)}; what runs and what is allocated has to fit it first.
						</p>
						<button
							type="button"
							class="btn mt-3"
							disabled={!!busy}
							onclick={() => changePlan(other.id)}
						>
							{busy === 'plan' ? 'Scheduling…' : `Downgrade to ${otherPlan.name}`}
						</button>
					{/if}
					{#if changeError}
						<p class="field-error" data-testid="change-error">{changeError}</p>
					{/if}
				</div>
			{/if}

			{#if confirmCancel}
				<div
					class="mt-4 rounded-sm border p-4 text-sm"
					style="border-color: var(--rule)"
					data-testid="confirm-cancel"
				>
					<p>
						Your plan ends on {dateOnly(
							sub.status === 'trialing' && sub.trial_end ? sub.trial_end : sub.period_end
						)}. Machines run until then and stop at it; snapshots are kept 30 days after. Nothing is
						charged after that.
					</p>
					<div class="mt-3 flex gap-2">
						<button type="button" class="btn-danger" disabled={!!busy} onclick={cancel}>
							{busy === 'cancel' ? 'Cancelling…' : 'Cancel plan'}
						</button>
						<button type="button" class="btn-ghost" onclick={() => (confirmCancel = false)}
							>Keep it</button
						>
					</div>
				</div>
			{/if}
		</section>

		<div class="form-section">
			<h2 class="font-display text-xl font-semibold">Invoices</h2>
			{#if invoices.length === 0}
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
					No invoices yet. The first comes with the first charge.
				</p>
			{:else}
				<ul class="mt-2" aria-label="Invoices">
					{#each invoices as inv (inv.id)}
						<li class="row flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-sm">
							<span>
								{dateOnly(inv.created_at)}
								{#if inv.number}· {inv.number}{/if}
								· <span class="badge">{inv.status}</span>
							</span>
							<span>
								{money(inv.amount_cents)}
								{#if inv.tax_cents > 0}
									<span class="text-zinc-500 dark:text-zinc-400">(tax {money(inv.tax_cents)})</span>
								{/if}
								{#if inv.pdf_url}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- Paddle's invoice PDF, not an app route -->
									<a href={inv.pdf_url} class="link ml-2">PDF</a>
								{:else if inv.hosted_url}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- Paddle's hosted invoice, not an app route -->
									<a href={inv.hosted_url} class="link ml-2">View</a>
								{/if}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
			<p class="mt-4 text-xs text-zinc-500 dark:text-zinc-400">
				Paddle is the merchant of record: receipts and tax come from Paddle.
				<a href={resolve('/refunds')} class="link">Refunds</a>.
			</p>
		</div>
	{/if}
</PageShell>

<style>
	/* Two plan cards side by side from 480px, one under the other below. */
	.plans {
		display: grid;
		gap: 1rem;
		list-style: none;
		margin: 0;
		padding: 0;
	}
	@media (min-width: 480px) {
		.plans {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
