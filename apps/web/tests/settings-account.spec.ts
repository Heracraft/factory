// Checklist: "Settings: timezone, email toggle, ntfy URL, test button." and
// "Account deletion flow requires typing the handle and explains
// retention."
import { test, expect } from '@playwright/test';
import { signIn } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('settings round-trips timezone, email toggle and ntfy URL, and the test button fires', async ({
	page
}) => {
	await page.goto('/settings');

	await page.getByLabel('ntfy URL').fill('https://ntfy.sh/repose-test');
	const emailToggle = page.getByRole('checkbox', { name: 'Email notifications' });
	await emailToggle.uncheck();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Settings saved.')).toBeVisible();

	await page.reload();
	await expect(page.getByLabel('ntfy URL')).toHaveValue('https://ntfy.sh/repose-test');
	await expect(page.getByRole('checkbox', { name: 'Email notifications' })).not.toBeChecked();

	await page.getByRole('button', { name: 'Send test' }).click();
	await expect(page.getByText(/Test notification/)).toBeVisible();
});

test('account deletion requires typing the exact handle', async ({ page }) => {
	await page.goto('/account');
	await expect(page.getByText('Everything, including snapshots, is deleted 30 days')).toBeVisible();

	const deleteBtn = page.getByRole('button', { name: 'Delete account' });
	await expect(deleteBtn).toBeDisabled();

	await page.getByPlaceholder(/to confirm/).fill('not-my-handle');
	await expect(deleteBtn).toBeDisabled();

	await page.getByPlaceholder(/to confirm/).fill('heracraft');
	await expect(deleteBtn).toBeEnabled();
	await deleteBtn.click();

	await expect(page).toHaveURL('/', { timeout: 10_000 });
});
