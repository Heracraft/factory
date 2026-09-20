// The config page's Menu tab selection state, pulled out as pure functions
// so it's unit-testable without mounting the component (08-dashboard.md §7
// "menu selection state").
import type { CatalogItem, MenuSelection } from './api/types';

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

export function buildMenuSelection(
	selectedPackages: Set<string>,
	selectedServices: Set<string>,
	options: Record<string, string>
): MenuSelection {
	return { packages: [...selectedPackages], services: [...selectedServices], options };
}
