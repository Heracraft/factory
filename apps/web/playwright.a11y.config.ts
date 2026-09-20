import type { PlaywrightTestConfig } from '@playwright/test';
import { BASE_URL } from './tests/fixtures';
import { CDP_PORT } from './tests-a11y/cdp';

// The Lighthouse run (08 §9's accessibility rows). Same fixtures as the
// fake-backed suite — tests/global-setup.ts starts cmd/fakeapi,
// cmd/fake-logto and `node build` — with one addition: Chromium is given a
// CDP port so the Lighthouse CLI can audit the pages this browser has
// already signed in to. Run `pnpm build` first, as for the other config.
const config: PlaywrightTestConfig = {
	testDir: 'tests-a11y',
	testMatch: /(.+\.)?spec\.ts/,
	globalSetup: './tests/global-setup.ts',
	use: {
		baseURL: BASE_URL,
		trace: 'retain-on-failure',
		launchOptions: {
			args: [`--remote-debugging-port=${CDP_PORT}`],
			...(process.env.PLAYWRIGHT_CHROMIUM_PATH
				? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH, args: [`--remote-debugging-port=${CDP_PORT}`, '--no-sandbox'] }
				: {})
		}
	},
	// One worker: one fixture process, one account, one debugging port.
	fullyParallel: false,
	workers: 1,
	timeout: 240_000
};

export default config;
