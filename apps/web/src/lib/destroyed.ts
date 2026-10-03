// Helpers for the "Recently destroyed" section of /projects (DECISIONS
// I-167): what name a restore proposes, and how long is left to restore.
import type { DestroyedProject } from './api/types';

/**
 * The name a restore proposes: the project's own name while no live
 * project holds it, else `<name>-restored` (and `-2`, `-3`… if that is
 * taken too). The user can edit it before restoring.
 */
export function defaultRestoreName(d: DestroyedProject, liveSlugs: Iterable<string>): string {
	if (d.name_free) return d.name;
	const taken = new Set(liveSlugs);
	const base = `${d.slug}-restored`;
	if (!taken.has(base)) return base;
	for (let i = 2; ; i++) {
		const candidate = `${base}-${i}`;
		if (!taken.has(candidate)) return candidate;
	}
}

/** "29 days left", "1 day left", "less than a day left" until `until`. */
export function timeLeft(until: string | null | undefined, now = new Date()): string {
	if (!until) return '';
	const ms = new Date(until).getTime() - now.getTime();
	if (ms <= 0) return 'expired';
	const days = Math.floor(ms / 86_400_000);
	if (days === 0) return 'less than a day left';
	return days === 1 ? '1 day left' : `${days} days left`;
}

/**
 * "Recently destroyed" shows the newest DESTROYED_FIRST rows and a "Show
 * more" that adds DESTROYED_STEP at a time (DECISIONS I-333). The api sends
 * DESTROYED_PAGE rows at a time; past them the page asks for the next
 * DESTROYED_PAGE (I-420).
 */
export const DESTROYED_FIRST = 10;
export const DESTROYED_STEP = 20;
export const DESTROYED_PAGE = 100;

/** How many more rows the next "Show more" reveals: 0 when all are shown. */
export function moreToShow(shown: number, total: number): number {
	return Math.max(0, Math.min(DESTROYED_STEP, total - shown));
}
