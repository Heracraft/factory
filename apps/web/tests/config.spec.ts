// Checklist: "Menu tab renders every catalog group and search filters it;
// Apply sends {menu} and shows the generated fragment." and "Nix tab:
// editor with Nix highlighting, Apply, build log streams, a failing
// fragment shows the error block with the line highlighted."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('menu tab renders catalog groups and search filters them', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'menu-app',
		remote_url: 'github.com/heracraft/menu-app'
	});
	await page.goto(`/projects/${p.id}/config`);
	await page.getByRole('button', { name: 'Menu' }).click();

	await expect(page.getByText('languages')).toBeVisible();
	await expect(page.getByText('Services')).toBeVisible();
	await expect(page.getByText('PostgreSQL', { exact: true })).toBeVisible();

	await page.getByPlaceholder('Search packages and services…').fill('postgres');
	await expect(page.getByText('PostgreSQL', { exact: true })).toBeVisible();
	await expect(page.getByText('Bun', { exact: true })).toHaveCount(0);
});

test('applying a menu selection shows the generated fragment on the Nix tab', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'menu-apply-app',
		remote_url: 'github.com/heracraft/menu-apply-app'
	});
	await page.goto(`/projects/${p.id}/config`);
	await page.getByRole('button', { name: 'Menu' }).click();
	await page.getByText('ripgrep', { exact: true }).click();
	await page.getByRole('button', { name: 'Apply', exact: true }).click();

	await expect(page.getByText('Applied.')).toBeVisible({ timeout: 10_000 });

	await page.getByRole('button', { name: 'Nix' }).click();
	await expect(page.locator('.cm-content')).toContainText('ripgrep');
});

test('a failing fragment shows the error block with the fragment line highlighted', async ({
	page
}) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'broken-fragment-app',
		remote_url: 'github.com/heracraft/broken-fragment-app'
	});
	await page.goto(`/projects/${p.id}/config`);
	await page.getByRole('button', { name: 'Nix' }).click();

	await page.locator('.cm-content').click();
	await page.keyboard.press('Control+A');
	await page.keyboard.type('{ pkgs, ... }: /* repose-force-eval-error */ { }');
	await page.getByRole('button', { name: 'Apply', exact: true }).click();

	await expect(page.getByText(/syntax error at fragment\.nix:1:32/)).toBeVisible({
		timeout: 10_000
	});
	await expect(page.locator('.cm-line-error')).toBeVisible();
});

test('hold base updates toggle round-trips', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'hold-app',
		remote_url: 'github.com/heracraft/hold-app'
	});
	await page.goto(`/projects/${p.id}/config`);

	const hold = page.getByRole('checkbox', { name: /Hold base updates/ });
	await expect(hold).not.toBeChecked();

	await hold.check();
	await expect(hold).toBeChecked();

	await page.reload();
	await expect(page.getByRole('checkbox', { name: /Hold base updates/ })).toBeChecked();
});
