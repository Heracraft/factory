// The project page's Events card shows the newest 20 and pages back with
// Show older until every event is shown (DECISIONS I-414).
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv, addEvents } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('Show older pages back through every event', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'busy-app',
		remote_url: 'github.com/heracraft/busy-app'
	});
	await addEvents(p.id, 75);
	await page.goto(`/projects/${p.id}`);
	const card = page.locator('.card', { has: page.getByRole('heading', { name: 'Events' }) });
	const rows = card.locator('li');
	// Creating the project adds its own two events, the newest.
	await expect(rows).toHaveCount(20);
	await expect(rows.nth(2)).toContainText('e074');
	const older = card.getByRole('button', { name: 'Show older' });
	// 40 from the poll's 50; 60 fetches the 27 before them; then 77.
	for (const n of [40, 60, 77]) {
		await older.click();
		await expect(rows).toHaveCount(n);
	}
	await expect(rows.last()).toContainText('e000');
	await expect(older).toHaveCount(0);
});

test('a project with few events has no Show older', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'calm-app',
		remote_url: 'github.com/heracraft/calm-app'
	});
	await addEvents(p.id, 5);
	await page.goto(`/projects/${p.id}`);
	const card = page.locator('.card', { has: page.getByRole('heading', { name: 'Events' }) });
	await expect(card.locator('li')).toHaveCount(7);
	await expect(card.getByRole('button', { name: 'Show older' })).toHaveCount(0);
});
