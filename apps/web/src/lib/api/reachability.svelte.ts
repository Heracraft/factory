// Backs the persistent "Cannot reach the API" bar (08-dashboard.md 5.5/6):
// the api client flips this on every request's success or NetworkError, and
// the root layout renders the bar off of it. Polling keeps running with
// backoff regardless (poll.ts already backs off to 60s once the tab is
// hidden; a network failure does not change the interval, it only changes
// what's on screen).
class Reachability {
	ok = $state(true);
}

export const reachability = new Reachability();
