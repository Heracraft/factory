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
