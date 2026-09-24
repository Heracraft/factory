import { test, expect } from '@playwright/test';

// /docs is public: a signed-out visitor stays on it, can move between pages
// and can search.
test('docs are readable while signed out, with search and prev/next', async ({ page }) => {
	await page.goto('/docs');
	await expect(page).toHaveURL('/docs');
	await expect(page.getByRole('heading', { level: 1, name: 'Quickstart' })).toBeVisible();

	await page.goto('/docs/machine');
	await expect(page).toHaveURL('/docs/machine');
	await expect(page.getByRole('heading', { level: 1, name: 'The machine' })).toBeVisible();
	await page.getByRole('link', { name: /Next\s*Config/ }).click();
	await expect(page).toHaveURL('/docs/config');

	await page.getByLabel('Search the docs').fill('ntfy topic');
	const results = page.getByRole('list', { name: 'Search results' });
	await results.getByRole('link').first().click();
	await expect(page).toHaveURL(/\/docs\/notifications/);
});

test('an unknown docs page says so', async ({ page }) => {
	await page.goto('/docs/no-such-page');
	await expect(page.getByRole('heading', { name: 'No such page' })).toBeVisible();
});
