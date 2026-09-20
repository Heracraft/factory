// Checklist: "Secrets: add via text and via file, list, delete; values
// never appear in the DOM after save."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('add via text, list, and delete; the value never reaches the DOM', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-app',
		remote_url: 'github.com/heracraft/secrets-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('DATABASE_URL');
	await page.getByPlaceholder('Value').fill('postgres://user:hunter2@db/app');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	const secretRow = page.getByText('DATABASE_URL', { exact: true });
	await expect(secretRow).toBeVisible({ timeout: 5_000 });
	expect(await page.content()).not.toContain('hunter2');

	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Delete' }).click();
	await expect(secretRow).toHaveCount(0, { timeout: 5_000 });
});

test('a name that does not match the allowed pattern is rejected client-side', async ({
	page
}) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-invalid-app',
		remote_url: 'github.com/heracraft/secrets-invalid-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('lowercase_name');
	await page.getByPlaceholder('Value').fill('x');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText(/\[A-Z\]\[A-Z0-9_\]/)).toBeVisible();
});

test('a value over 64 KB is rejected client-side before it reaches the api', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-huge-app',
		remote_url: 'github.com/heracraft/secrets-huge-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('BIG_VALUE');
	await page.getByPlaceholder('Value').fill('x'.repeat(70_000));
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText('Secret values are limited to 64 KB.')).toBeVisible();
	await expect(page.getByText('BIG_VALUE')).toHaveCount(0);
});
