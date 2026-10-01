// The browser the Lighthouse runs share with the test.
//
// Lighthouse, attached with --port, opens its own tab with a plain
// Target.createTarget, which lands in the browser's default context.
// Playwright's usual `page` lives in an isolated context of its own, whose
// localStorage (where the Logto SDK keeps the session) that tab cannot see,
// so an audit of /projects from there is an audit of the landing page the
// signed-out layout redirects to. A persistent context is the default
// context, so the tab Lighthouse opens shares the session. Playwright also
// attaches to that tab as one of its own and applies the context's
// colour-scheme emulation to it, which is how the dark runs reach
// prefers-color-scheme: dark. Both were checked against Lighthouse 13.5
// before this file was written: a probe page reported its localStorage and
// matchMedia result back through its URL, and only the persistent context
// passed both.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test as base, type BrowserContext } from '@playwright/test';
import { BASE_URL } from '../tests/fixtures';

export type Scheme = 'light' | 'dark';

interface WorkerFixtures {
	/** The colour scheme this project audits, set per project in the config. */
	scheme: Scheme;
	a11yContext: BrowserContext;
}

export const test = base.extend<object, WorkerFixtures>({
	scheme: ['light', { option: true, scope: 'worker' }],
	a11yContext: [
		async ({ playwright, launchOptions, scheme }, use) => {
			// A fresh profile per worker, so every worker starts signed out and
			// the public pages are audited as a visitor sees them.
			const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'repose-a11y-'));
			const context = await playwright.chromium.launchPersistentContext(dir, {
				...launchOptions,
				baseURL: BASE_URL,
				colorScheme: scheme,
				// No viewport override: Lighthouse sets the audited tab's size
				// itself, and a second override from Playwright would race it.
				viewport: null
			});
			await use(context);
			await context.close();
			fs.rmSync(dir, { recursive: true, force: true });
		},
		{ scope: 'worker' }
	],
	// The test's page is the persistent context's own, so signing in on it
	// signs in the tab Lighthouse opens next to it.
	page: async ({ a11yContext }, use) => {
		await use(a11yContext.pages()[0] ?? (await a11yContext.newPage()));
	}
});

export { expect } from '@playwright/test';
