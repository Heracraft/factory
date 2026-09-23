import { describe, expect, it } from 'vitest';
import { defaultRestoreName, timeLeft } from './destroyed';
import type { DestroyedProject } from './api/types';

function destroyed(over: Partial<DestroyedProject> = {}): DestroyedProject {
	return {
		id: 'p1',
		name: 'Izma',
		slug: 'izma',
		class: 'large',
		volume_bytes: 40 * 2 ** 30,
		destroyed_at: '2026-09-23T02:23:23Z',
		name_free: true,
		restorable_until: '2026-10-23T02:23:19Z',
		snapshot: { id: 's1', created_at: '2026-09-23T02:23:19Z', bytes: 2_059_646, reason: 'stop' },
		...over
	};
}

describe('defaultRestoreName', () => {
	it('keeps the name while it is free', () => {
		expect(defaultRestoreName(destroyed(), [])).toBe('Izma');
	});
	it('proposes <slug>-restored when a live project holds the name', () => {
		expect(defaultRestoreName(destroyed({ name_free: false }), ['izma'])).toBe('izma-restored');
	});
	it('counts past names that are taken too', () => {
		expect(
			defaultRestoreName(destroyed({ name_free: false }), [
				'izma',
				'izma-restored',
				'izma-restored-2'
			])
		).toBe('izma-restored-3');
	});
});

describe('timeLeft', () => {
	const now = new Date('2026-09-23T03:00:00Z');
	it('counts whole days', () => {
		expect(timeLeft('2026-10-23T02:23:19Z', now)).toBe('29 days left');
		expect(timeLeft('2026-09-24T12:00:00Z', now)).toBe('1 day left');
	});
	it('says when less than a day is left or it has gone', () => {
		expect(timeLeft('2026-09-23T20:00:00Z', now)).toBe('less than a day left');
		expect(timeLeft('2026-09-22T00:00:00Z', now)).toBe('expired');
		expect(timeLeft(null, now)).toBe('');
	});
});
