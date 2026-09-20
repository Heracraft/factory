import { describe, it, expect } from 'vitest';
import { encodeBase64 } from './base64';

describe('encodeBase64', () => {
	it('matches btoa for plain ASCII', () => {
		expect(encodeBase64('hello')).toBe(btoa('hello'));
	});

	it('is UTF-8 safe, unlike bare btoa', () => {
		// btoa throws on non-Latin1 text; this must not.
		expect(() => encodeBase64('héllo 🚀')).not.toThrow();
		const decoded = new TextDecoder().decode(
			Uint8Array.from(atob(encodeBase64('héllo 🚀')), (c) => c.charCodeAt(0))
		);
		expect(decoded).toBe('héllo 🚀');
	});

	it('round-trips an ArrayBuffer the same way', () => {
		const bytes = new Uint8Array([0, 1, 2, 255]);
		const decoded = Uint8Array.from(atob(encodeBase64(bytes.buffer)), (c) => c.charCodeAt(0));
		expect([...decoded]).toEqual([0, 1, 2, 255]);
	});
});
