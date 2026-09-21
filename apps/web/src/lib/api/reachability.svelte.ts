// Backs the persistent "Cannot reach the API" bar (08-dashboard.md 5.5/6):
// the api client flips this on every request's success or NetworkError, and
// the root layout renders the bar off of it.
//
// It also changes the polling interval, which this comment used to deny:
// §6 says polling "continues with backoff to 60 s" while the api is
// unreachable, and `pollWhileVisible` reads `ok` to decide between 10s and
// 60s. Pinned by poll.test.ts "backs off to 60 s while the api is
// unreachable, even when visible", so the two cannot drift apart again.
class Reachability {
	ok = $state(true);
}

export const reachability = new Reachability();
