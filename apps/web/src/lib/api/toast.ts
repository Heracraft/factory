// Error rendering per 08-dashboard.md 5.5/6: every api error is a toast with
// its message; a couple of codes get a different, call-site-specific
// treatment (payment_required and capacity on Start) instead of a toast.
import { toast } from 'svelte-sonner';
import { errorText } from './errors';

export function toastApiError(err: unknown, fallback = 'Something went wrong.'): void {
	toast.error(errorText(err, fallback));
}
