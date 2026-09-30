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
 */
export class PollFailure {
	private toastId: string | number | undefined;

	constructor(private fallback: string) {}

	fail(err: unknown, quiet = false): void {
		if (this.toastId !== undefined) return;
		if (quiet || !reachability.ok) return;
		this.toastId = toast.error(errorText(err, this.fallback));
	}

	ok(): void {
		if (this.toastId !== undefined) toast.dismiss(this.toastId);
		this.toastId = undefined;
	}
}
