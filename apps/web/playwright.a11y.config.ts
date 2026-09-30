import type { PlaywrightTestConfig } from '@playwright/test';
import { BASE_URL } from './tests/fixtures';
import { CDP_PORT } from './tests-a11y/cdp';
import type { Scheme } from './tests-a11y/fixtures';

// The Lighthouse run (08 §9's accessibility rows). Same fixtures as the
// fake-backed suite (tests/global-setup.ts starts cmd/fakeapi,
// cmd/fake-logto and `node build`), with one addition: Chromium is given a
// CDP port so the Lighthouse CLI can audit the pages this browser has
// already signed in to. Run `pnpm build` first, as for the other config.
const config: PlaywrightTestConfig<object, { scheme: Scheme }> = {
	testDir: 'tests-a11y',
	testMatch: /(.+\.)?spec\.ts/,
	globalSetup: './tests/global-setup.ts',
	// One project per colour scheme. The dark tokens are a second palette
	// with contrast pairs of their own, and an audit in light says nothing
	// about them.
	projects: [
		{ name: 'light', use: { scheme: 'light' } },
		{ name: 'dark', use: { scheme: 'dark' } }
	],
	use: {
		baseURL: BASE_URL,
		trace: 'retain-on-failure',
		launchOptions: {
			// The debugging port is the whole point of this config; --no-sandbox
			// rides with the Nix chromium, which cannot use the sandbox here.
			args: process.env.PLAYWRIGHT_CHROMIUM_PATH
				? [`--remote-debugging-port=${CDP_PORT}`, '--no-sandbox']
				: [`--remote-debugging-port=${CDP_PORT}`],
			...(process.env.PLAYWRIGHT_CHROMIUM_PATH
				? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH }
				: {})
		}
	},
	// One worker: one fixture process, one account, one debugging port.
	// The two projects therefore run one after the other, each in a browser
	// of its own.
	fullyParallel: false,
	workers: 1,
	timeout: 240_000
};

export default config;
