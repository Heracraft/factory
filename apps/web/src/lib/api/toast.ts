// Error rendering per 08-dashboard.md 5.5/6: every api error is a toast with
// its message; a couple of codes get a different, call-site-specific
// treatment (payment_required and capacity on Start) instead of a toast.
import { toast } from 'svelte-sonner';
import { ApiError, NetworkError } from './errors';

export function toastApiError(err: unknown, fallback = 'Something went wrong.'): void {
	if (err instanceof ApiError) {
		toast.error(err.message);
		return;
	}
	if (err instanceof NetworkError) {
		toast.error('Cannot reach the API.');
		return;
	}
	toast.error(fallback);
}
