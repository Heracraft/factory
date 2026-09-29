import { describe, it, expect } from 'vitest';
import {
	money,
	gb,
	gbs,
	price,
	dateOnly,
	timeUntil,
	tempLeft,
	share,
	normalizeRemoteDisplay
} from './format';

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

describe('price', () => {
	it('drops the cents of a whole-dollar plan price', () => {
		expect(price(2900)).toBe('$29');
		expect(price(5900)).toBe('$59');
	});
	it('keeps the cents of an overage', () => {
		expect(price(250)).toBe('$2.50');
	});
});

describe('gbs', () => {
	it("formats the api's GB figures", () => {
		expect(gbs(100)).toBe('100 GB');
		expect(gbs(37.25)).toBe('37.3 GB');
		expect(gbs(0)).toBe('0 GB');
	});
});

describe('dateOnly', () => {
	it('gives the local date of a charge', () => {
		const d = new Date(2026, 9, 4, 15, 30);
		expect(dateOnly(d.toISOString())).toBe('2026-10-04');
	});
});

describe('timeUntil', () => {
	const now = new Date(2026, 8, 27, 12, 0, 0);
	it('counts hours under two days and days after', () => {
		expect(timeUntil(new Date(2026, 8, 27, 13, 30).toISOString(), now)).toBe('1 hour');
		expect(timeUntil(new Date(2026, 8, 28, 12, 0).toISOString(), now)).toBe('24 hours');
		expect(timeUntil(new Date(2026, 8, 30, 12, 0).toISOString(), now)).toBe('3 days');
	});
	it('says less than an hour, then nothing once passed', () => {
		expect(timeUntil(new Date(2026, 8, 27, 12, 20).toISOString(), now)).toBe('less than an hour');
		expect(timeUntil(new Date(2026, 8, 27, 11, 0).toISOString(), now)).toBe('');
	});
});

describe('tempLeft', () => {
	const now = new Date(2026, 8, 27, 12, 0, 0);
	it('rounds up to hours, minutes under an hour, as the CLI does', () => {
		expect(tempLeft(new Date(2026, 8, 27, 17, 0).toISOString(), now)).toBe('destroyed in 5h');
		expect(tempLeft(new Date(2026, 8, 27, 16, 1).toISOString(), now)).toBe('destroyed in 5h');
		expect(tempLeft(new Date(2026, 8, 27, 12, 30).toISOString(), now)).toBe('destroyed in 30m');
		expect(tempLeft(new Date(2026, 8, 27, 11, 0).toISOString(), now)).toBe('time is up');
	});
});

describe('share', () => {
	it('clamps a usage bar to the limit and treats no limit as empty', () => {
		expect(share(4, 8)).toBe(0.5);
		expect(share(300, 250)).toBe(1);
		expect(share(3, 0)).toBe(0);
		expect(share(-1, 8)).toBe(0);
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
