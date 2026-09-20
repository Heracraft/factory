// Checklist: "Every failure row in §6 is exercised."
import { test, expect, type Page } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv, failNext, fail, unfail } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

// A freshly created project starts already running (the fake's create
// path), so the Start tests stop it first. Wait for the page to render its
// action button before looking: an `isVisible()` on a page still loading
// is false, the stop is skipped, and the Start click then waits on a
// button that never appears (CI, 2026-09-20). The one-shot failure rule is
// armed only after the stop so that a failed run leaves nothing behind for
// the next test to trip on.
async function stopIfRunning(page: Page): Promise<void> {
	const action = page.getByRole('button', { name: /^(Start|Stop)$/ });
	await expect(action.first()).toBeVisible({ timeout: 10_000 });
	if (await page.getByRole('button', { name: 'Stop' }).isVisible()) {
		await page.getByRole('button', { name: 'Stop' }).click();
	}
	await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeVisible({
		timeout: 10_000
	});
}

test('Start with no card shows an inline banner and the button stays enabled', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'no-card-app',
		remote_url: 'github.com/heracraft/no-card-app'
	});
	await page.goto(`/projects/${p.id}`);
	await stopIfRunning(page);
	await failNext('POST', '/projects/:id/start', 'payment_required');

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
	await stopIfRunning(page);
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
