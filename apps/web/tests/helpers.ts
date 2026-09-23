import type { Page } from '@playwright/test';
import { BASE_URL } from './fixtures';

/** Drives the whole fake-Logto authorization-code round trip. */
export async function signIn(page: Page): Promise<void> {
	await page.goto('/');
	await page.click('text=Sign in with GitHub');
	await page.waitForURL(/\/oidc\/auth/);
	await page.getByRole('button', { name: 'Continue as heracraft' }).click();
	await page.waitForURL(/\/projects/);
}

/**
 * Creates a project directly against the fake api (no CLI in this suite).
 * The name and remote get a random suffix so a retried test, or a test
 * file that runs more than once in the same fixture process, never hits
 * the (user, name) uniqueness conflict.
 */
export async function createProject(
	apiURL: string,
	body: { name: string; remote_url: string; class?: string }
): Promise<{ id: string; slug: string; name: string }> {
	const suffix = Math.random().toString(36).slice(2, 8);
	const res = await fetch(`${apiURL}/projects`, {
		method: 'POST',
		headers: { Authorization: 'Bearer playwright', 'Content-Type': 'application/json' },
		body: JSON.stringify({
			class: 'large',
			...body,
			name: `${body.name}-${suffix}`,
			remote_url: `${body.remote_url}-${suffix}`
		})
	});
	if (!res.ok) throw new Error(`createProject: ${res.status} ${await res.text()}`);
	return res.json();
}

export function apiURLFromEnv(): string {
	const url = process.env.PUBLIC_API_URL;
	if (!url) throw new Error('PUBLIC_API_URL not set — run tests through global-setup.ts');
	return url;
}

/** Drives the fake api's error switch (cmd/fakeapi's admin listener). */
async function adminCall(route: string, body: { method: string; path: string; code?: string }) {
	const adminURL = process.env.FAKEAPI_ADMIN_URL;
	if (!adminURL) throw new Error('FAKEAPI_ADMIN_URL not set — run tests through global-setup.ts');
	const res = await fetch(`${adminURL}${route}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`${route}: ${res.status}`);
}

export const failNext = (method: string, path: string, code: string) =>
	adminCall('/fail-next', { method, path, code });
export const fail = (method: string, path: string, code: string) =>
	adminCall('/fail', { method, path, code });
export const unfail = (method: string, path: string) => adminCall('/unfail', { method, path });

/** Switches the fake's billing routes: off (503), card, or nocard (trial, no card). */
export async function setBilling(mode: 'off' | 'card' | 'nocard'): Promise<void> {
	const adminURL = process.env.FAKEAPI_ADMIN_URL;
	if (!adminURL) throw new Error('FAKEAPI_ADMIN_URL not set — run tests through global-setup.ts');
	const res = await fetch(`${adminURL}/billing`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ mode })
	});
	if (!res.ok) throw new Error(`/billing: ${res.status}`);
}

export { BASE_URL };
