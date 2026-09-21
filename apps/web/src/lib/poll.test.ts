// 08-dashboard.md §5.3: the projects list and project detail poll every
// 10s while the tab is visible and every 60s otherwise, ops poll every 2s
// until done, and both stop when told to. §6 adds the backoff: while the
// api is unreachable, polling "continues with backoff to 60 s".
//
// A unit test rather than a Playwright one: the thing under test is an
// interval, and asserting an interval end to end means either a slow test
// or a flaky one. Fake timers make "after exactly 10 seconds" mean it.
import { describe, expect, test, vi, beforeEach, afterEach } from 'vitest';
import { pollWhileVisible, pollUntilDone } from './poll';
import { reachability } from './api/reachability.svelte';

vi.mock('$app/environment', () => ({ browser: true }));

/** jsdom reports "visible" and has no API to change it. */
function setVisibility(state: 'visible' | 'hidden') {
	Object.defineProperty(document, 'visibilityState', {
		configurable: true,
		get: () => state
	});
	document.dispatchEvent(new Event('visibilitychange'));
}

describe('pollWhileVisible', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		setVisibility('visible');
		reachability.ok = true;
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	test('calls immediately, then every 10 s while visible', async () => {
		const fn = vi.fn();
		const stop = pollWhileVisible(fn);
		expect(fn).toHaveBeenCalledTimes(1); // immediately, not after the first interval

		await vi.advanceTimersByTimeAsync(9_999);
		expect(fn).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(fn).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(3);
		stop();
	});

	test('polls every 60 s while the tab is hidden', async () => {
		const fn = vi.fn();
		setVisibility('hidden');
		const stop = pollWhileVisible(fn);
		expect(fn).toHaveBeenCalledTimes(1);

		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(1); // not the visible interval
		await vi.advanceTimersByTimeAsync(50_000);
		expect(fn).toHaveBeenCalledTimes(2);
		stop();
	});

	// §6: "API 5xx or unreachable | persistent bar, polling continues with
	// backoff to 60 s". The bar is the layout's job; the backoff is this.
	test('backs off to 60 s while the api is unreachable, even when visible', async () => {
		const fn = vi.fn();
		reachability.ok = false;
		const stop = pollWhileVisible(fn);

		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(50_000);
		expect(fn).toHaveBeenCalledTimes(2);
		stop();
	});

	test('a visibility change reschedules against the new interval at once', async () => {
		const fn = vi.fn();
		const stop = pollWhileVisible(fn);
		expect(fn).toHaveBeenCalledTimes(1);

		// Five seconds into a 10 s wait, the tab is hidden: the remaining
		// five must not fire, and the next poll is 60 s away, not 5 s.
		await vi.advanceTimersByTimeAsync(5_000);
		setVisibility('hidden');
		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(50_000);
		expect(fn).toHaveBeenCalledTimes(2);
		stop();
	});

	test('stop() ends the polling and removes the listener', async () => {
		const fn = vi.fn();
		const stop = pollWhileVisible(fn);
		stop();

		await vi.advanceTimersByTimeAsync(120_000);
		expect(fn).toHaveBeenCalledTimes(1); // only the immediate one

		// A visibility change after stopping must not resurrect it.
		setVisibility('hidden');
		await vi.advanceTimersByTimeAsync(120_000);
		expect(fn).toHaveBeenCalledTimes(1);
	});

	test('waits for a slow poll rather than overlapping calls', async () => {
		let resolve!: () => void;
		const fn = vi.fn(() => new Promise<void>((r) => (resolve = r)));
		const stop = pollWhileVisible(fn);
		expect(fn).toHaveBeenCalledTimes(1);

		// The first call has not resolved, so no interval is running yet.
		await vi.advanceTimersByTimeAsync(60_000);
		expect(fn).toHaveBeenCalledTimes(1);

		resolve();
		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(2);
		stop();
	});
});

describe('pollUntilDone', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	test('polls every 2 s until the callback reports done', async () => {
		const fn = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(false).mockResolvedValue(true);
		pollUntilDone(fn);
		expect(fn).toHaveBeenCalledTimes(1);

		await vi.advanceTimersByTimeAsync(2_000);
		expect(fn).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(2_000);
		expect(fn).toHaveBeenCalledTimes(3); // returned true here

		await vi.advanceTimersByTimeAsync(10_000);
		expect(fn).toHaveBeenCalledTimes(3); // and stopped
	});

	test('stop() ends it mid-flight', async () => {
		const fn = vi.fn().mockResolvedValue(false);
		const stop = pollUntilDone(fn);
		await vi.advanceTimersByTimeAsync(2_000);
		expect(fn).toHaveBeenCalledTimes(2);

		stop();
		await vi.advanceTimersByTimeAsync(20_000);
		expect(fn).toHaveBeenCalledTimes(2);
	});
});
