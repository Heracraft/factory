import { describe, it, expect } from 'vitest';
import { money, gb, projectedMonthCents, normalizeRemoteDisplay } from './format';

describe('money', () => {
	it('formats cents as dollars', () => {
		expect(money(9900)).toBe('$99.00');
		expect(money(0)).toBe('$0.00');
		expect(money(31)).toBe('$0.31');
	});
});

describe('gb', () => {
	it('formats whole GB without a decimal', () => {
		expect(gb(40 * 1024 * 1024 * 1024)).toBe('40 GB');
	});
	it('formats a fraction with one decimal', () => {
		expect(gb(1.5 * 1024 * 1024 * 1024)).toBe('1.5 GB');
	});
});

describe('projectedMonthCents', () => {
	it('projects a flat run rate across the rest of the month', () => {
		// Noon on January 1st (31 days): 12h elapsed today and this month,
		// 744 total hours, so 732 remain. $12 so far today is a $1/h rate,
		// so the projection is today's $12 plus $1/h for the 732 remaining
		// hours.
		const now = new Date(2026, 0, 1, 12, 0, 0);
		const projected = projectedMonthCents(1200, 1200, now);
		expect(projected).toBe(1200 + 100 * 732);
	});

	it('returns the month-to-date figure when less than an hour has elapsed today', () => {
		const now = new Date(2026, 0, 5, 0, 30, 0);
		expect(projectedMonthCents(5000, 0, now)).toBe(5000);
	});
});

describe('normalizeRemoteDisplay', () => {
	it('normalizes an ssh-style remote', () => {
		expect(normalizeRemoteDisplay('git@github.com:heracraft/repose.git')).toBe(
			'github.com/heracraft/repose'
		);
	});

	it('normalizes an https remote to the same form', () => {
		expect(normalizeRemoteDisplay('https://GitHub.com/heracraft/repose')).toBe(
			'github.com/heracraft/repose'
		);
	});

	it('agrees on both spellings of the same remote', () => {
		const a = normalizeRemoteDisplay('git@github.com:a/b.git');
		const b = normalizeRemoteDisplay('https://github.com/a/b');
		expect(a).toBe(b);
	});
});
