// Checklist: "Every route in 5.2 exists and renders with the fake API."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv } from './helpers';

let projectId: string;
let projectName: string;

test.beforeAll(async () => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'routes-app',
		remote_url: 'github.com/heracraft/routes-app'
	});
	projectId = p.id;
	projectName = p.name;
});

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('/projects renders the projects list', async ({ page }) => {
	await page.goto('/projects');
	await expect(page.getByText(projectName)).toBeVisible();
});

test('/projects/[id] renders the project detail cards', async ({ page }) => {
	await page.goto(`/projects/${projectId}`);
	await expect(page.getByRole('heading', { name: projectName })).toBeVisible();
	await expect(page.getByText('Connect')).toBeVisible();
	await expect(page.getByText('Signals')).toBeVisible();
	await expect(page.getByText('Cost')).toBeVisible();
	await expect(page.getByText('Disk')).toBeVisible();
	await expect(page.getByText('Events')).toBeVisible();
	await expect(page.getByText('Snapshots', { exact: true })).toBeVisible();
	await expect(page.getByText('Last build')).toBeVisible();
});

test('/projects/[id]/config renders the Menu and Nix tabs', async ({ page }) => {
	await page.goto(`/projects/${projectId}/config`);
	await expect(page.getByRole('button', { name: 'Menu' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Nix' })).toBeVisible();
});

test('/projects/[id]/secrets renders the secrets page', async ({ page }) => {
	await page.goto(`/projects/${projectId}/secrets`);
	await expect(page.getByText('Add a secret')).toBeVisible();
});

test('/billing renders', async ({ page }) => {
	await page.goto('/billing');
	await expect(page.getByRole('heading', { name: 'Billing' })).toBeVisible();
	await expect(page.getByText('Usage this month')).toBeVisible();
});

test('/settings renders', async ({ page }) => {
	await page.goto('/settings');
	await expect(page.getByText('Timezone')).toBeVisible();
	await expect(page.getByText('Notifications', { exact: true })).toBeVisible();
});

test('/account renders', async ({ page }) => {
	await page.goto('/account');
	await expect(page.getByText('heracraft').first()).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Delete account' })).toBeVisible();
});

test('/terms and /privacy render', async ({ page }) => {
	await page.goto('/terms');
	await expect(page.locator('article')).toBeVisible();
	await page.goto('/privacy');
	await expect(page.locator('article')).toBeVisible();
	await expect(page.getByText('Draft:')).toBeVisible();
});

test('/healthz answers 200', async ({ request }) => {
	const res = await request.get('/healthz');
	expect(res.status()).toBe(200);
});
