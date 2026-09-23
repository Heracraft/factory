import { describe, it, expect } from 'vitest';
import {
	toggleSelection,
	groupCatalog,
	buildMenuSelection,
	parseMenuSelection
} from './menuSelection';
import type { CatalogItem } from './api/types';

const CATALOG: CatalogItem[] = [
	{ id: 'bun', label: 'Bun', group: 'languages', kind: 'package', description: 'JS runtime' },
	{
		id: 'postgresql',
		label: 'PostgreSQL',
		group: 'databases',
		kind: 'service',
		description: 'a database'
	},
	{ id: 'ripgrep', label: 'ripgrep', group: 'tools', kind: 'package', description: 'fast grep' }
];

describe('toggleSelection', () => {
	it('adds an id when checked', () => {
		const next = toggleSelection(new Set(), 'bun', true);
		expect([...next]).toEqual(['bun']);
	});

	it('removes an id when unchecked', () => {
		const next = toggleSelection(new Set(['bun', 'ripgrep']), 'bun', false);
		expect([...next]).toEqual(['ripgrep']);
	});

	it('never mutates the set it was given', () => {
		const original = new Set(['bun']);
		toggleSelection(original, 'ripgrep', true);
		expect([...original]).toEqual(['bun']);
	});
});

describe('groupCatalog', () => {
	it('pulls service-kind items into a Services group regardless of their catalog group', () => {
		const groups = groupCatalog(CATALOG, '');
		expect(groups.get('databases')).toBeUndefined();
		expect(groups.get('Services')?.map((i) => i.id)).toEqual(['postgresql']);
		expect(groups.get('languages')?.map((i) => i.id)).toEqual(['bun']);
	});

	it('filters by label or description, case-insensitively', () => {
		expect([...groupCatalog(CATALOG, 'POSTGRES').values()].flat().map((i) => i.id)).toEqual([
			'postgresql'
		]);
		expect([...groupCatalog(CATALOG, 'fast').values()].flat().map((i) => i.id)).toEqual([
			'ripgrep'
		]);
		expect(groupCatalog(CATALOG, 'nonexistent').size).toBe(0);
	});
});

describe('buildMenuSelection', () => {
	it('assembles the documented [{id, options?} | {package}] array in catalog order', () => {
		const withOpts: CatalogItem[] = [
			...CATALOG,
			{
				id: 'nodejs',
				label: 'Node.js',
				group: 'runtimes',
				kind: 'runtime',
				description: 'node',
				options: [{ id: 'version', type: 'enum', values: ['20', '22'], default: '22' }]
			}
		];
		const sel = buildMenuSelection(
			withOpts,
			new Set(['nodejs', 'postgresql', 'bun']),
			{ nodejs: '20' },
			['gcc', 'python312Packages.black']
		);
		expect(sel).toEqual([
			{ id: 'bun' },
			{ id: 'postgresql' },
			{ id: 'nodejs', options: { version: '20' } },
			{ package: 'gcc' },
			{ package: 'python312Packages.black' }
		]);
	});
});

describe('parseMenuSelection', () => {
	it('splits catalog ids, their option values and extra packages', () => {
		expect(
			parseMenuSelection([
				{ id: 'bun' },
				{ id: 'nodejs', options: { version: '20' } },
				{ package: 'gcc' }
			])
		).toEqual({ ids: ['bun', 'nodejs'], options: { nodejs: '20' }, packages: ['gcc'] });
	});

	it('reads anything that is not the documented array as empty', () => {
		const empty = { ids: [], options: {}, packages: [] };
		expect(parseMenuSelection(null)).toEqual(empty);
		expect(parseMenuSelection({ packages: ['bun'] })).toEqual(empty);
	});
});
