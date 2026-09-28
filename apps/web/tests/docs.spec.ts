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
	await page.getByRole('link', { name: /Next\s*Installing software/ }).click();
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

// A block with prompts copies its commands only, without the `$ ` or the
// output under them.
test('a code block copies its commands', async ({ page, context }) => {
	await context.grantPermissions(['clipboard-read', 'clipboard-write']);
	await page.goto('/docs/lifecycle');
	const block = page.locator('.doc .code').filter({ hasText: '$ repose stop todo-app' });
	await block.getByRole('button', { name: 'Copy' }).click();
	await expect(block.getByRole('button', { name: 'Copied' })).toBeVisible();
	expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
		'repose stop todo-app\nrepose start todo-app'
	);
});
