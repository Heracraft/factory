import type { ApiErrorBody, ErrorCode } from './types';

/** An error envelope the api returned, per docs/interfaces/api.md. */
export class ApiError extends Error {
	code: ErrorCode;
	detail?: Record<string, unknown>;
	requestId: string | null;
	status: number;

	constructor(body: ApiErrorBody, status: number, requestId: string | null) {
		super(body.message);
		this.name = 'ApiError';
		this.code = body.code;
		this.detail = body.detail;
		this.requestId = requestId;
		this.status = status;
	}
}

/** The api could not be reached at all: DNS, TLS, connection refused, offline. */
export class NetworkError extends Error {
	constructor(cause: unknown) {
		super('Cannot reach the API');
		this.name = 'NetworkError';
		this.cause = cause;
	}
}

export function isApiError(e: unknown, code?: ErrorCode): e is ApiError {
	return e instanceof ApiError && (code === undefined || e.code === code);
}

/**
 * The sentence a failure is shown with, in a toast or a page's load banner,
 * so one failure reads the same in both. The api's own message is a whole
 * sentence for the codes a person can act on; `internal` carries only
 * "internal error", which says nothing to the reader, so it gets the
 * caller's sentence and what happened instead.
 */
export function errorText(err: unknown, fallback: string): string {
	if (err instanceof ApiError) {
		if (err.code === 'internal')
			return `${fallback} The API failed on its side; try again shortly.`;
		return err.message;
	}
	if (err instanceof NetworkError) return 'Cannot reach the API.';
	return fallback;
}

/**
 * Whether a response means the api is in trouble, for the persistent bar
 * (08-dashboard.md 6, "API 5xx or unreachable"). Two codes come with a 503
 * on purpose and are answers, not outages: `billing_disabled` (billing is
 * not switched on) and `waitlisted` (no free seat). Counting them put the
 * outage bar over /billing whenever billing was off (DECISIONS I-390).
 */
export function isOutage(status: number, code: ErrorCode | undefined): boolean {
	if (status < 500) return false;
	return code !== 'billing_disabled' && code !== 'waitlisted';
}
