import { describe, it, expect } from 'vitest';
import { ApiError, NetworkError, errorText, isApiError, isOutage } from './errors';

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

describe('errorText', () => {
	it('uses the api sentence, except for internal, which says nothing to a reader', () => {
		const refused = new ApiError({ code: 'invalid', message: 'Name is taken.' }, 400, null);
		expect(errorText(refused, 'Could not save.')).toBe('Name is taken.');
		const internal = new ApiError({ code: 'internal', message: 'internal error' }, 500, null);
		expect(errorText(internal, 'Could not load projects.')).toBe(
			'Could not load projects. The API failed on its side; try again shortly.'
		);
		expect(errorText(new NetworkError(new Error('x')), 'Could not save.')).toBe(
			'Cannot reach the API.'
		);
		expect(errorText(new Error('boom'), 'Could not save.')).toBe('Could not save.');
	});
});

describe('isOutage', () => {
	it('counts a 5xx as an outage unless it is an answer the api chose to give', () => {
		expect(isOutage(500, 'internal')).toBe(true);
		expect(isOutage(502, undefined)).toBe(true);
		expect(isOutage(503, 'billing_disabled')).toBe(false);
		expect(isOutage(503, 'waitlisted')).toBe(false);
		// Start refused for want of a host is the capacity banner's answer.
		expect(isOutage(503, 'capacity')).toBe(false);
		// A bare 503 from a proxy, with no envelope, is an outage.
		expect(isOutage(503, undefined)).toBe(true);
		// Only a 503 is an answer: the same code on a 500 means trouble.
		expect(isOutage(500, 'capacity')).toBe(true);
		expect(isOutage(404, 'not_found')).toBe(false);
	});
});
