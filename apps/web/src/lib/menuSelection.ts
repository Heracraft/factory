// The config page's Menu tab selection state, pulled out as pure functions
// so it's unit-testable without mounting the component (08-dashboard.md §7
// "menu selection state").
import type { CatalogItem, MenuItem, MenuSelection } from './api/types';

/** Adds or removes `id` from `selected`, returning a new Set (never mutates). */
export function toggleSelection(selected: Set<string>, id: string, checked: boolean): Set<string> {
	const next = new Set(selected);
	if (checked) next.add(id);
	else next.delete(id);
	return next;
}

/** The catalog groups a menu tab renders: service-kind items under "Services",
 * everything else under the catalog's own `group`, filtered by a search term. */
export function groupCatalog(catalog: CatalogItem[], search: string): Map<string, CatalogItem[]> {
	const q = search.trim().toLowerCase();
	const byGroup = new Map<string, CatalogItem[]>();
	for (const item of catalog) {
		if (q && !item.label.toLowerCase().includes(q) && !item.description.toLowerCase().includes(q)) {
			continue;
		}
		const group = item.kind === 'service' ? 'Services' : item.group;
		if (!byGroup.has(group)) byGroup.set(group, []);
		byGroup.get(group)!.push(item);
	}
	return byGroup;
}

/** What the menu tab shows for a stored selection: the catalog ids that are
 * ticked, the value of each id's (single) option, and the nixpkgs packages
 * added by name (`repose config add gcc`), which the tab lists as "Extra
 * packages". Anything that is not the documented array reads as empty. */
export interface MenuState {
	ids: string[];
	options: Record<string, string>;
	packages: string[];
}

export function parseMenuSelection(sel: unknown): MenuState {
	const state: MenuState = { ids: [], options: {}, packages: [] };
	if (!Array.isArray(sel)) return state;
	for (const item of sel as MenuItem[]) {
		if (item && 'package' in item && typeof item.package === 'string') {
			state.packages.push(item.package);
		} else if (item && 'id' in item && typeof item.id === 'string') {
			state.ids.push(item.id);
			const first = Object.values(item.options ?? {})[0];
			if (first !== undefined) state.options[item.id] = first;
		}
	}
	return state;
}

/** The PUT /config `menu` array: selected catalog entries in catalog order
 * (with their option when the entry has one), then the extra packages. */
export function buildMenuSelection(
	catalog: CatalogItem[],
	selected: Set<string>,
	options: Record<string, string>,
	packages: string[]
): MenuSelection {
	const sel: MenuSelection = [];
	for (const item of catalog) {
		if (!selected.has(item.id)) continue;
		const opt = item.options?.[0];
		const value = options[item.id];
		sel.push(
			opt && value !== undefined ? { id: item.id, options: { [opt.id]: value } } : { id: item.id }
		);
	}
	for (const p of packages) sel.push({ package: p });
	return sel;
}
