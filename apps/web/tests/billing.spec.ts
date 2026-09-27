// docs/workstreams/08-dashboard.md §5.8 and DECISIONS I-289/I-290: the
// billing page sells two plans through Paddle, gates them on seats, and
// shows a subscription's status, usage of the plan and its invoices.
import { test, expect } from '@playwright/test';
import {
	signIn,
	setBilling,
	resetBilling,
	installPaddleStub,
	createProject,
	apiURLFromEnv
} from './helpers';

test.afterAll(async () => {
	await resetBilling();
});

test.beforeEach(async ({ page }) => {
	await resetBilling();
	await signIn(page);
});

test('with billing off the page says so and sells nothing', async ({ page }) => {
	await page.goto('/billing');
	await expect(page.getByTestId('billing-disabled')).toHaveText('Billing is not switched on yet.');
	await expect(page.getByRole('button', { name: /Choose/ })).toHaveCount(0);
});

test('with no plan and seats free, both plan cards are shown from GET /billing', async ({
	page
}) => {
	await setBilling({ mode: 'none' });
	await page.goto('/billing');
	await expect(page.getByTestId('seats-line')).toHaveText('18 of 30 seats left.');
	const solo = page.getByTestId('plan-solo');
	await expect(solo.getByRole('heading', { name: 'Solo' })).toBeVisible();
	await expect(solo.getByText('$29')).toBeVisible();
	await expect(solo.getByText('8 GB: one large, or two small')).toBeVisible();
	await expect(solo.getByText('100 GB', { exact: true })).toBeVisible();
	await expect(solo.getByText('250 GB', { exact: true })).toBeVisible();
	await expect(solo.getByText('7 days free, card at checkout, cancel any time.')).toBeVisible();
	const pro = page.getByTestId('plan-pro');
	await expect(pro.getByText('$59')).toBeVisible();
	await expect(pro.getByText('16 GB: one xl, two large, or any mix')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Choose Solo' })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Choose Pro' })).toBeEnabled();
	await expect(page.getByRole('link', { name: 'Refunds' })).toHaveAttribute('href', '/refunds');
});

test('one seat free: Solo can be chosen, Pro says why not', async ({ page }) => {
	await setBilling({ mode: 'none', seats: { total: 30, held: 29, waiting: 0 } });
	await page.goto('/billing');
	await expect(page.getByRole('button', { name: 'Choose Solo' })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Choose Pro' })).toBeDisabled();
	await expect(page.getByText('Needs 2 seats; 1 free.')).toBeVisible();
});

test('choosing a plan opens the checkout and, once completed, the plan appears', async ({
	page
}) => {
	await setBilling({ mode: 'none' });
	await installPaddleStub(page);
	await page.goto('/billing');
	await page.getByRole('button', { name: 'Choose Pro' }).click();
	// The stub completed the transaction; the page polls GET /billing.
	await expect(page.getByTestId('plan').getByRole('heading', { name: 'Pro' })).toBeVisible({
		timeout: 10_000
	});
	const opened = await page.evaluate(() => window.__reposePaddleOpened);
	expect(opened).toMatch(/^txn_fake_/);
	await expect(page.getByTestId('plan-status')).toContainText('Trial. First charge of $59 on');
	await expect(page.getByTestId('meter-running-now')).toContainText('0 GB of 16 GB');
	await expect(
		page.getByText('No invoices yet. The first comes with the first charge.')
	).toBeVisible();
});

test('coming back on ?checkout=done waits for the plan', async ({ page }) => {
	await setBilling({ mode: 'none' });
	await page.goto('/billing?checkout=done');
	await expect(page.getByTestId('setting-up')).toContainText('Setting up your plan');
	expect(new URL(page.url()).searchParams.get('checkout')).toBeNull();
	// The webhook lands while the page polls.
	await setBilling({ mode: 'trial', plan: 'solo' });
	await expect(page.getByTestId('plan').getByRole('heading', { name: 'Solo' })).toBeVisible({
		timeout: 10_000
	});
});

test('when repose is full the page offers the waitlist and then shows the place', async ({
	page
}) => {
	await setBilling({ mode: 'none', seats: { total: 30, held: 30, waiting: 40 } });
	await page.goto('/billing');
	const full = page.getByTestId('full');
	await expect(full.getByRole('heading', { name: 'repose is full' })).toBeVisible();
	await expect(full).toContainText('All 30 seats are taken and 40 people are waiting.');
	await expect(page.getByRole('button', { name: /Choose/ })).toHaveCount(0);
	await page.getByTestId('join-waitlist').click();
	const place = page.getByTestId('waitlist-place');
	await expect(place).toContainText("You're number 41 on the waitlist.");
	await expect(place).toContainText(
		"We'll email dev@example.com when a seat frees; you'll have 72 hours to choose a plan."
	);
	// Reloading shows the same place: the api remembers it.
	await page.reload();
	await expect(page.getByTestId('waitlist-place')).toContainText('number 41');
});

test('an invited user sees the held seat and the plan cards', async ({ page }) => {
	await setBilling({
		mode: 'none',
		seats: { total: 30, held: 30, waiting: 12 },
		waitlist: { position: 1, invited: true, hold_hours: 70 }
	});
	await page.goto('/billing');
	await expect(page.getByTestId('seat-held')).toContainText('Your seat is held until');
	await expect(page.getByTestId('seat-held')).toContainText('(2 days left)');
	await expect(page.getByRole('button', { name: 'Choose Solo' })).toBeEnabled();
	// Pro needs two seats; the hold is one.
	await expect(page.getByRole('button', { name: 'Choose Pro' })).toBeDisabled();
});

test('a trial shows the first charge date, the usage bars and the project count', async ({
	page
}) => {
	await setBilling({ mode: 'trial', plan: 'solo', egress_gb: 300 });
	await page.goto('/billing');
	await expect(page.getByTestId('plan-status')).toContainText('Trial. First charge of $29 on');
	await expect(page.getByTestId('meter-disk-allocated')).toContainText('of 100 GB');
	const egress = page.getByTestId('meter-egress-this-period');
	await expect(egress).toContainText('300 GB of 250 GB');
	await expect(egress).toContainText('Over by 50 GB: $2.50 on the next invoice at $0.05 a GB.');
	await expect(page.getByTestId('projects-count')).toContainText('of 10');
});

test('an active plan shows its renewal, receipts through Paddle, and invoices with PDF links', async ({
	page
}) => {
	await setBilling({ mode: 'active', plan: 'pro' });
	await page.goto('/billing');
	await expect(page.getByTestId('plan-status')).toContainText('Active. Renews');
	const list = page.getByRole('list', { name: 'Invoices' });
	await expect(list.getByText('$59.00')).toBeVisible();
	await expect(list.getByText('REPOSE-0001', { exact: false })).toBeVisible();
	await expect(list.getByRole('link', { name: 'PDF' })).toHaveAttribute(
		'href',
		'https://checkout.paddle.com/invoice/txn_fake_inv_000001.pdf'
	);
	await page.route('https://customer-portal.paddle.com/**', (route) =>
		route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Paddle portal</h1>' })
	);
	await page.getByRole('button', { name: /Manage card and receipts/ }).click();
	await page.waitForURL('https://customer-portal.paddle.com/cpl_fake');
	await expect(page.getByRole('heading', { name: 'Paddle portal' })).toBeVisible();
});

test('past due shows the failed payment and the card link; suspended says what happened', async ({
	page
}) => {
	await setBilling({ mode: 'past_due', plan: 'solo' });
	await page.goto('/billing');
	await expect(page.getByTestId('status-past-due')).toContainText('Your last payment failed.');
	await page.route('https://customer-portal.paddle.com/**', (route) =>
		route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Update card</h1>' })
	);
	await page.getByRole('button', { name: 'Update card' }).click();
	await page.waitForURL(/update-payment-method$/);

	await setBilling({ mode: 'suspended', plan: 'solo' });
	await page.goto('/billing');
	await expect(page.getByTestId('status-suspended')).toContainText('Your account is suspended');
	await expect(page.getByTestId('status-suspended')).toContainText('snapshots are kept 30 days');
});

test('upgrading takes effect at once; a downgrade the machines do not fit is refused with over_plan', async ({
	page
}) => {
	await setBilling({ mode: 'active', plan: 'solo' });
	await page.goto('/billing');
	await page.getByRole('button', { name: 'Change plan' }).click();
	const change = page.getByTestId('change-plan');
	await expect(change).toContainText('Upgrade to Pro ($59 a month');
	await expect(change).toContainText('Takes effect at once');
	await page.getByRole('button', { name: 'Upgrade to Pro' }).click();
	await expect(page.getByTestId('plan').getByRole('heading', { name: 'Pro' })).toBeVisible();

	// Two large machines running: 16 GB, Pro's whole allowance and twice Solo's.
	const api = apiURLFromEnv();
	await createProject(api, {
		name: 'over-a',
		remote_url: 'github.com/heracraft/over-a',
		class: 'large'
	});
	await createProject(api, {
		name: 'over-b',
		remote_url: 'github.com/heracraft/over-b',
		class: 'large'
	});
	await page.reload();
	await expect(page.getByTestId('meter-running-now')).toContainText('16 GB of 16 GB');
	await page.getByRole('button', { name: 'Change plan' }).click();
	await expect(page.getByTestId('change-plan')).toContainText('Downgrade to Solo ($29 a month');
	await page.getByRole('button', { name: 'Downgrade to Solo' }).click();
	const err = page.getByTestId('change-error');
	await expect(err).toContainText('Solo holds 8 GB running at once and 100 GB of disk');
	await expect(err).toContainText('you have 16 GB running');
	await expect(err).toContainText('Stop machines or destroy projects first.');
});

test('a downgrade that fits is scheduled for the renewal and can be undone', async ({ page }) => {
	// The seat count and plan are set directly; the fake's own projects
	// (created by other specs) are what the gate measures, so this test
	// stops them all first through the api.
	const api = process.env.PUBLIC_API_URL!;
	const list = (await (
		await fetch(`${api}/projects`, { headers: { Authorization: 'Bearer playwright' } })
	).json()) as { id: string; state: string }[];
	for (const p of list) {
		if (p.state === 'running') {
			await fetch(`${api}/projects/${p.id}/stop`, {
				method: 'POST',
				headers: { Authorization: 'Bearer playwright', 'Content-Type': 'application/json' },
				body: JSON.stringify({ snapshot: false })
			});
		}
	}
	await setBilling({ mode: 'active', plan: 'pro' });
	await page.goto('/billing');
	await page.getByRole('button', { name: 'Change plan' }).click();
	await page.getByRole('button', { name: 'Downgrade to Solo' }).click();
	await expect(page.getByTestId('plan-status')).toContainText('Changes to Solo on');
	await page.getByRole('button', { name: 'Change plan' }).click();
	await expect(page.getByTestId('change-plan')).toContainText('Solo is scheduled for');
	await page.getByRole('button', { name: 'Keep Pro' }).click();
	await expect(page.getByTestId('plan-status')).not.toContainText('Changes to Solo');
});

test('cancelling asks first, then shows the end date and a Resume that undoes it', async ({
	page
}) => {
	await setBilling({ mode: 'active', plan: 'solo' });
	await page.goto('/billing');
	await page.getByRole('button', { name: 'Cancel plan' }).click();
	const confirm = page.getByTestId('confirm-cancel');
	await expect(confirm).toContainText('Your plan ends on');
	await expect(confirm).toContainText('snapshots are kept 30 days after');
	await page.getByRole('button', { name: 'Keep it' }).click();
	await expect(page.getByTestId('confirm-cancel')).toHaveCount(0);
	await page.getByRole('button', { name: 'Cancel plan' }).click();
	await page.getByTestId('confirm-cancel').getByRole('button', { name: 'Cancel plan' }).click();
	await expect(page.getByTestId('plan-status')).toContainText('Cancelled. Ends');
	await expect(page.getByRole('button', { name: 'Change plan' })).toHaveCount(0);
	await page.getByRole('button', { name: 'Resume plan' }).click();
	await expect(page.getByTestId('plan-status')).toContainText('Active. Renews');
});
