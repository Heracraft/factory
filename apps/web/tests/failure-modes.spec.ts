// Checklist: "Every failure row in §6 is exercised."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv, failNext, fail, unfail } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('Start with no card shows an inline banner and the button stays enabled', async ({
	page
}) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'no-card-app',
		remote_url: 'github.com/heracraft/no-card-app'
	});
	await failNext('POST', '/projects/:id/start', 'payment_required');
	await page.goto(`/projects/${p.id}`);
	// A freshly created project starts already running (the fake's create
	// path), so stop it first to reach the Start button.
	if (await page.getByRole('button', { name: 'Stop' }).isVisible()) {
		await page.getByRole('button', { name: 'Stop' }).click();
		await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeVisible({
			timeout: 10_000
		});
	}

	const startBtn = page.getByRole('button', { name: 'Start', exact: true });
	await startBtn.click();
	await expect(page.getByText('Add one in Billing')).toBeVisible();
	await expect(startBtn).toBeEnabled();
});

test('capacity on Start shows the documented message', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'capacity-app',
		remote_url: 'github.com/heracraft/capacity-app'
	});
	await page.goto(`/projects/${p.id}`);
	if (await page.getByRole('button', { name: 'Stop' }).isVisible()) {
		await page.getByRole('button', { name: 'Stop' }).click();
		await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeVisible({
			timeout: 10_000
		});
	}
	await failNext('POST', '/projects/:id/start', 'capacity');
	await page.getByRole('button', { name: 'Start', exact: true }).click();
	await expect(page.getByText('No capacity right now, try again in a few minutes.')).toBeVisible();
});

test('a 5xx from the api shows the persistent "cannot reach" bar, which clears once the api recovers', async ({
	page
}) => {
	await fail('GET', '/projects', 'internal');
	await page.goto('/projects');
	await expect(page.getByText('Cannot reach the API')).toBeVisible({ timeout: 15_000 });

	await unfail('GET', '/projects');
	// The next poll is scheduled up to 60s out once unreachable (backoff);
	// reloading forces an immediate re-check rather than waiting it out.
	await page.reload();
	await expect(page.getByText('Cannot reach the API')).toHaveCount(0, { timeout: 10_000 });
});
