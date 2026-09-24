import type { Project } from './api/types';

/**
 * The api's sentence for a project the platform stopped because a
 * cryptocurrency miner was running (DECISIONS I-239), from last_error
 * "abuse_stopped: stopped: a cryptocurrency miner (xmrig) was running; ...".
 * Empty for any other project, including a stopped one whose last_error is
 * an older, unrelated failure.
 */
export function abuseStopReason(p: Pick<Project, 'state' | 'last_error'>): string {
	if (p.state !== 'stopped' || !p.last_error) return '';
	const prefix = 'abuse_stopped: ';
	return p.last_error.startsWith(prefix) ? p.last_error.slice(prefix.length) : '';
}
