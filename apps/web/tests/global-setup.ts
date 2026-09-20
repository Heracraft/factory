import { startAll, stopAll } from './fixtures';

// Returning the teardown function from globalSetup (rather than a separate
// globalTeardown config entry) keeps the fixture processes' handles in one
// process's module state, since Playwright guarantees this returned
// function runs in the same process that ran globalSetup.
export default async function globalSetup() {
	await startAll();
	return async () => {
		await stopAll();
	};
}
