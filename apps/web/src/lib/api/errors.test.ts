import { describe, it, expect } from 'vitest';
import { ApiError, NetworkError, isApiError } from './errors';

describe('ApiError', () => {
	it('carries the documented envelope fields plus the transport ones', () => {
		const err = new ApiError(
			{ code: 'payment_required', message: 'card required', detail: { reason: 'card_required' } },
			402,
			'req-000123'
		);
		expect(err.code).toBe('payment_required');
		expect(err.message).toBe('card required');
		expect(err.detail).toEqual({ reason: 'card_required' });
		expect(err.status).toBe(402);
		expect(err.requestId).toBe('req-000123');
		expect(err).toBeInstanceOf(Error);
	});
});

describe('isApiError', () => {
	it('is true for any ApiError when no code is given', () => {
		const err = new ApiError({ code: 'not_found', message: 'not found' }, 404, null);
		expect(isApiError(err)).toBe(true);
	});

	it('narrows to a specific code', () => {
		const err = new ApiError({ code: 'capacity', message: 'no capacity' }, 503, null);
		expect(isApiError(err, 'capacity')).toBe(true);
		expect(isApiError(err, 'payment_required')).toBe(false);
	});

	it('is false for a plain Error or a NetworkError', () => {
		expect(isApiError(new Error('boom'))).toBe(false);
		expect(isApiError(new NetworkError(new TypeError('fetch failed')))).toBe(false);
	});
});

describe('NetworkError', () => {
	it('keeps the underlying cause and a fixed message', () => {
		const cause = new TypeError('fetch failed');
		const err = new NetworkError(cause);
		expect(err.message).toBe('Cannot reach the API');
		expect(err.cause).toBe(cause);
	});
});
