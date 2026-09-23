// Checklist: "Start, Stop, Destroy, Resize call the right routes and show
// op progress."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('stop and start round trip', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'lifecycle-app',
		remote_url: 'github.com/heracraft/lifecycle-app'
	});
	await page.goto(`/projects/${p.id}`);
	await expect(page.getByText('running', { exact: true })).toBeVisible();

	await page.getByRole('button', { name: 'Stop' }).click();
	await expect(page.getByText('stopped', { exact: true })).toBeVisible({ timeout: 10_000 });

	await page.getByRole('button', { name: 'Start', exact: true }).click();
	await expect(page.getByText('running', { exact: true })).toBeVisible({ timeout: 10_000 });
});

test('resize grows the volume', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'resize-app',
		remote_url: 'github.com/heracraft/resize-app',
		class: 'small'
	});
	await page.goto(`/projects/${p.id}`);
	await expect(page.getByText('/ 20 GB')).toBeVisible();

	await page.getByRole('button', { name: 'Resize…' }).click();
	await page.getByRole('combobox').selectOption('40');
	await page.getByRole('button', { name: 'Grow' }).click();
	await expect(page.getByText('/ 40 GB')).toBeVisible({ timeout: 10_000 });
});

test('destroy requires the exact slug and redirects to the projects list', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'destroy-app',
		remote_url: 'github.com/heracraft/destroy-app'
	});
	await page.goto(`/projects/${p.id}`);

	const destroyBtn = page.getByRole('button', { name: 'Destroy', exact: true });
	await expect(destroyBtn).toBeDisabled();

	await page.getByPlaceholder(/to confirm/).fill('wrong-slug');
	await expect(destroyBtn).toBeDisabled();

	await page.getByPlaceholder(/to confirm/).fill(p.slug);
	await expect(destroyBtn).toBeEnabled();
	await destroyBtn.click();

	await expect(page).toHaveURL('/projects', { timeout: 10_000 });
});

// DECISIONS I-167: a destroyed project is listed under "Recently destroyed"
// with its snapshot and expiry, and Restore brings it back as a new
// project under the same name.
test('a destroyed project can be restored from the projects list', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'restore-app',
		remote_url: 'github.com/heracraft/restore-app'
	});
	await page.goto(`/projects/${p.id}`);
	await page.getByPlaceholder(/to confirm/).fill(p.slug);
	await page.getByRole('button', { name: 'Destroy', exact: true }).click();
	await expect(page).toHaveURL('/projects', { timeout: 10_000 });

	const section = page.getByRole('region', { name: 'Recently destroyed' });
	await expect(section).toBeVisible({ timeout: 15_000 });
	const row = section.getByTestId('destroyed-row').filter({ hasText: p.name });
	await expect(row).toContainText('restorable until');
	await expect(row).toContainText('days left');

	await row.getByRole('button', { name: 'Restore…' }).click();
	await expect(row.getByLabel('Name for the restored project')).toHaveValue(p.name);
	await row.getByRole('button', { name: 'Restore', exact: true }).click();

	await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+$/, { timeout: 10_000 });
	await expect(page).not.toHaveURL(`/projects/${p.id}`);
	await expect(page.getByRole('heading', { name: p.name })).toBeVisible();
});
