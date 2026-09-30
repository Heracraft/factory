// I-393: one failure, one report. A poll that fails on every tick toasts
// once, says nothing while a banner or the outage bar already says it, and
// takes its toast away when a poll gets through.
import { describe, expect, test, vi, beforeEach } from 'vitest';

const { error, dismiss } = vi.hoisted(() => ({
	error: vi.fn(() => 'toast-1'),
	dismiss: vi.fn()
}));
vi.mock('svelte-sonner', () => ({ toast: { error, dismiss } }));

import { PollFailure, PollGroup } from './toast';
import { ApiError } from './errors';
import { reachability } from './reachability.svelte';

const forbidden = new ApiError({ code: 'forbidden', message: 'Not yours.' }, 403, null);

describe('PollFailure', () => {
	beforeEach(() => {
		error.mockClear();
		dismiss.mockClear();
		reachability.ok = true;
	});

	test('toasts on the first failure only, and again after a recovery', () => {
		const f = new PollFailure('Could not load events.');
		f.fail(forbidden);
		f.fail(forbidden);
		f.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(1);
		expect(error).toHaveBeenCalledWith('Not yours.');

		f.ok();
		expect(dismiss).toHaveBeenCalledWith('toast-1');
		f.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(2);
	});

	test('says nothing while the page banner shows', () => {
		const f = new PollFailure('Could not load snapshots.');
		f.fail(forbidden, true);
		f.fail(forbidden, true);
		expect(error).not.toHaveBeenCalled();
		f.ok();
		expect(dismiss).not.toHaveBeenCalled();
	});

	test('toasts once the banner has gone and the poll still fails', () => {
		// The project page's events poll: its first tick fails under the load
		// banner (quiet), Retry loads the project, and events keeps failing.
		const f = new PollFailure('Could not load events.');
		f.fail(forbidden, true);
		f.fail(forbidden);
		f.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(1);
	});

	test('says nothing while the outage bar shows, and toasts if the poll fails after it', () => {
		const f = new PollFailure('Could not load projects.');
		reachability.down('server');
		f.fail(new ApiError({ code: 'internal', message: 'internal error' }, 500, null));
		expect(error).not.toHaveBeenCalled();
		reachability.ok = true;
		f.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(1);
	});

	test('polls in one group raise one toast, gone when all of them get through', () => {
		// The project page on a 429: its three polls fail on the same tick.
		const page = new PollGroup();
		const project = new PollFailure('Could not load the project.', page);
		const events = new PollFailure('Could not load events.', page);
		const snapshots = new PollFailure('Could not load snapshots.', page);
		project.fail(forbidden);
		events.fail(forbidden);
		snapshots.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(1);

		project.ok();
		events.ok();
		expect(dismiss).not.toHaveBeenCalled();
		snapshots.ok();
		expect(dismiss).toHaveBeenCalledWith('toast-1');

		events.fail(forbidden);
		expect(error).toHaveBeenCalledTimes(2);
	});
});
