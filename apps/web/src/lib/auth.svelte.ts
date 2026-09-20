// Logto SPA login (docs/workstreams/08-dashboard.md 5.1): authorization code
// with PKCE entirely in the browser, access tokens for the api resource,
// refresh in the client. No server route ever sees a token or a secret.
import { browser } from '$app/environment';
import { env } from '$env/dynamic/public';
import { goto } from '$app/navigation';
import { toast } from 'svelte-sonner';
import LogtoClient from '@logto/browser';

/** The api's resource identifier: PUBLIC_API_URL without its /v1 suffix. */
export function apiResource(): string {
	return (env.PUBLIC_API_URL ?? '').replace(/\/v1\/?$/, '');
}

let client: LogtoClient | undefined;

function getClient(): LogtoClient {
	if (!browser) throw new Error('the Logto client only runs in the browser');
	if (!client) {
		client = new LogtoClient({
			endpoint: env.PUBLIC_LOGTO_ENDPOINT ?? '',
			appId: env.PUBLIC_LOGTO_APP_ID ?? '',
			resources: [apiResource()],
			scopes: ['openid', 'profile', 'email', 'offline_access']
		});
	}
	return client;
}

class AuthState {
	// undefined until the first isAuthenticated() check resolves, so the
	// layout can show nothing (rather than flash the landing page) while
	// Logto reads its stored refresh token.
	authenticated = $state<boolean | undefined>(undefined);
}

export const authState = new AuthState();

export async function initAuth(): Promise<void> {
	if (!browser) return;
	authState.authenticated = await getClient().isAuthenticated();
}

export function callbackUrl(): string {
	return `${location.origin}/callback`;
}

export async function signIn(): Promise<void> {
	await getClient().signIn(callbackUrl());
}

export async function handleSignInCallback(url: string): Promise<void> {
	await getClient().handleSignInCallback(url);
	authState.authenticated = true;
}

export async function signOut(): Promise<void> {
	authState.authenticated = false;
	await getClient().signOut(`${location.origin}/`);
}

/**
 * Signs the user out locally and sends them to the landing page without a
 * round trip to Logto's own end-session endpoint, for the "refresh failed"
 * failure mode (08-dashboard.md 6): the refresh token is already dead, so
 * there is nothing for Logto to revoke.
 */
async function forceSignOutExpired(): Promise<void> {
	authState.authenticated = false;
	toast.error('Session expired, sign in again.');
	await goto('/');
}

/** Used by the api client. Throws if the user is signed out or refresh failed. */
export async function getAccessToken(): Promise<string> {
	try {
		const token = await getClient().getAccessToken(apiResource());
		if (!token) throw new Error('no access token');
		return token;
	} catch (err) {
		await forceSignOutExpired();
		throw err;
	}
}
