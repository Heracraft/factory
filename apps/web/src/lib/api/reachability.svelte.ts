// Backs the persistent bar at the top of every page (08-dashboard.md
// 5.5/6): the api client calls down() on a network failure or a 5xx that is
// an outage (isOutage in errors.ts) and up() on any other answer, and the
// root layout renders the bar off of it. `reason` picks the bar's words: a
// request that never got an answer is "Cannot reach the API"; a 500 did get
// one, so saying the api cannot be reached would be wrong (DECISIONS I-390).
//
// It also changes the polling interval, which this comment used to deny:
// §6 says polling "continues with backoff to 60 s" while the api is
// unreachable, and `pollWhileVisible` reads `ok` to decide between 10s and
// 60s. Pinned by poll.test.ts "backs off to 60 s while the api is
// unreachable, even when visible", so the two cannot drift apart again.
export type Outage = 'network' | 'server';

class Reachability {
	ok = $state(true);
	reason = $state<Outage>('network');

	down(reason: Outage): void {
		this.reason = reason;
		this.ok = false;
	}

	up(): void {
		this.ok = true;
	}
}

export const reachability = new Reachability();
