// Polling helpers for 08-dashboard.md 5.3: the projects list and project
// detail poll every 10s while the tab is visible, 60s otherwise, and stop
// when the page/component goes away. Ops poll every 2s until done.
import { browser } from '$app/environment';
import { reachability } from './api/reachability.svelte';

/**
 * Calls `fn` immediately, then again after 10s (tab visible) or 60s (tab
 * hidden or the api unreachable — 6 "polling continues with backoff to
 * 60s"), for as long as the tab exists. Call the returned function to stop
 * (from a component's `$effect` cleanup, typically): that is what "stops
 * when the tab is closed" means in practice, since a closed tab runs no
 * JavaScript to keep polling with.
 */
export function pollWhileVisible(fn: () => void | Promise<void>): () => void {
	if (!browser) return () => {};
	let stopped = false;
	let timer: ReturnType<typeof setTimeout> | undefined;

	const schedule = () => {
		if (stopped) return;
		const ms = document.visibilityState === 'visible' && reachability.ok ? 10_000 : 60_000;
		timer = setTimeout(tick, ms);
	};
	const tick = async () => {
		if (stopped) return;
		await fn();
		schedule();
	};

	void tick();
	document.addEventListener('visibilitychange', onVisibilityChange);

	// A visibility change reschedules immediately against the new interval,
	// rather than waiting out whatever was left of the old one.
	function onVisibilityChange() {
		if (stopped || !timer) return;
		clearTimeout(timer);
		schedule();
	}

	return () => {
		stopped = true;
		if (timer) clearTimeout(timer);
		document.removeEventListener('visibilitychange', onVisibilityChange);
	};
}

/**
 * Calls `fn` every 2s until it returns true (the op reached a terminal
 * state), per 5.3 "Ops (start, stop, build) poll GET /ops/:op every 2
 * seconds until done."
 */
export function pollUntilDone(fn: () => boolean | Promise<boolean>): () => void {
	if (!browser) return () => {};
	let stopped = false;
	let timer: ReturnType<typeof setTimeout> | undefined;

	const tick = async () => {
		if (stopped) return;
		const done = await fn();
		if (done || stopped) return;
		timer = setTimeout(tick, 2_000);
	};
	void tick();

	return () => {
		stopped = true;
		if (timer) clearTimeout(timer);
	};
}
