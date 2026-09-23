// 09-billing.md §5.2 and DECISIONS I-182/I-183: the card goes on file
// through Stripe's hosted Checkout page (no publishable key in the
// dashboard), the invoices list reads the documented names, and the
// customer portal is one click away.
import { test, expect } from '@playwright/test';
import { signIn, setBilling } from './helpers';

test.afterAll(async () => {
	await setBilling('off');
});

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('a trial account adds its card through Stripe Checkout and comes back to it on file', async ({
	page
}) => {
	await setBilling('nocard');
	await page.goto('/billing');
	await expect(page.getByText('$10.00 trial credit remaining.')).toBeVisible();
	await page.getByRole('button', { name: 'Add a card' }).click();
	// The fake's Checkout URL is the success_url Stripe would send the user
	// back to, and the fake has applied the card the way the webhook would.
	await page.waitForURL(/\/billing/);
	await expect(page.getByText('Card saved.')).toBeVisible();
	await expect(page.getByText('A card is on file.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Add a card' })).toHaveCount(0);
	expect(new URL(page.url()).searchParams.get('card')).toBeNull();
});

test('coming back from a cancelled Checkout says no card was added', async ({ page }) => {
	await setBilling('nocard');
	await page.goto('/billing?card=cancelled');
	await expect(page.getByText('No card was added.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Add a card' })).toBeVisible();
});

test('invoices show the amount, number and Stripe links', async ({ page }) => {
	await setBilling('card');
	await page.goto('/billing');
	const list = page.getByRole('list', { name: 'Invoices' });
	await expect(list.getByText('$8.00')).toBeVisible();
	await expect(list.getByText('REPOSE-0001', { exact: false })).toBeVisible();
	await expect(list.getByRole('link', { name: 'View' })).toHaveAttribute(
		'href',
		'https://invoice.stripe.com/i/fake'
	);
	await expect(list.getByRole('link', { name: 'PDF' })).toHaveAttribute(
		'href',
		'https://pay.stripe.com/invoice/fake/pdf'
	);
});

test('the portal button opens the Stripe customer portal', async ({ page }) => {
	await setBilling('card');
	await page.route('https://billing.stripe.com/**', (route) =>
		route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Stripe portal</h1>' })
	);
	await page.goto('/billing');
	await page.getByRole('button', { name: /Manage card, address and invoices/ }).click();
	await page.waitForURL('https://billing.stripe.com/p/session/fake');
	await expect(page.getByRole('heading', { name: 'Stripe portal' })).toBeVisible();
});

test('with billing off the page says so and offers no card form', async ({ page }) => {
	await setBilling('off');
	await page.goto('/billing');
	await expect(page.getByText('Billing is not enabled for this account yet.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Add a card' })).toHaveCount(0);
});
