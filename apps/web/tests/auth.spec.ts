import { test, expect } from '@playwright/test';
import { signIn } from './helpers';

test('sign-in, callback and sign-out round trip', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('button', { name: 'Sign in with GitHub' })).toBeVisible();

	await signIn(page);
	await expect(page).toHaveURL(/\/projects/);
	await expect(page.getByRole('link', { name: 'repose' })).toBeVisible();

	await page.getByRole('button', { name: 'Sign out' }).click();
	await expect(page).toHaveURL('/');
	await expect(page.getByRole('button', { name: 'Sign in with GitHub' })).toBeVisible();
});

test('visiting a protected route while signed out redirects to the landing page', async ({
	page
}) => {
	await page.goto('/settings');
	await expect(page).toHaveURL('/');
});

test('a cancelled or failed sign-in shows a toast and stays on the landing page', async ({
	page
}) => {
	await page.goto('/?error=access_denied');
	await expect(page.getByText('Sign-in was cancelled or failed; try again.')).toBeVisible();
	await expect(page).toHaveURL('/');
});

test('terms and privacy are reachable while signed out', async ({ page }) => {
	await page.goto('/terms');
	await expect(page).toHaveURL('/terms');
	await page.goto('/privacy');
	await expect(page).toHaveURL('/privacy');
});
