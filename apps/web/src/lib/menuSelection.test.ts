import { describe, it, expect } from 'vitest';
import { toggleSelection, groupCatalog, buildMenuSelection } from './menuSelection';
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
	it('assembles the documented {packages, services, options} shape', () => {
		const sel = buildMenuSelection(new Set(['bun']), new Set(['postgresql']), {
			nodejs: '24'
		});
		expect(sel).toEqual({ packages: ['bun'], services: ['postgresql'], options: { nodejs: '24' } });
	});
});
