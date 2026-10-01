// Error rendering per 08-dashboard.md 5.5/6: every api error is a toast with
// its message; a couple of codes get a different, call-site-specific
// treatment (payment_required and capacity on Start) instead of a toast.
import { toast } from 'svelte-sonner';
import { errorText } from './errors';
import { reachability } from './reachability.svelte';

export function toastApiError(err: unknown, fallback = 'Something went wrong.'): void {
	toast.error(errorText(err, fallback));
}

/**
 * Reports a poll's failures once (DESIGN-LANGUAGE.md "Failure", DECISIONS
 * I-393). A poll that toasted on every failed tick said one outage every 10
 * or 60 seconds, and three polls on one page said it three times over. The
 * toast comes on the first failure only, and not at all when something on
 * screen already says it: the page's load banner (`quiet`) or the outage
 * bar, which the client raised before the error reached here. The first
 * poll that gets through dismisses the toast, so a stale one does not stay
 * after the api is back.
 *
 * Only a toast latches. A failure that stayed quiet is looked at again on
 * the next tick: if the banner or the bar has gone and the poll still
 * fails, that tick toasts. Latching the quiet one too left a project's
 * events list silently stale when its first tick failed under the load
 * banner and later ticks kept failing after Retry cleared it.
 *
 * Polls on one page share a PollGroup, so a failure that reaches all of
 * them at once, such as a 429 or a 403 on one tick of the project page,
 * is one toast, not one per poll (I-395). The toast names the first poll
 * that failed and goes when every poll it covers has got through again.
 */
export class PollFailure {
	constructor(
		private fallback: string,
		private group = new PollGroup()
	) {}

	fail(err: unknown, quiet = false): void {
		this.group.fail(this, errorText(err, this.fallback), quiet);
	}

	ok(): void {
		this.group.ok(this);
	}
}

/** One toast for the polls on a page; see PollFailure. */
export class PollGroup {
	private toastId: string | number | undefined;
	private failing = new Set<PollFailure>();

	fail(poll: PollFailure, text: string, quiet: boolean): void {
		if (quiet || !reachability.ok) return;
		this.failing.add(poll);
		if (this.toastId === undefined) this.toastId = toast.error(text);
	}

	ok(poll: PollFailure): void {
		this.failing.delete(poll);
		if (this.failing.size > 0 || this.toastId === undefined) return;
		toast.dismiss(this.toastId);
		this.toastId = undefined;
	}
}
