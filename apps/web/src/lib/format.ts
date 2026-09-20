/** "$1.23" from a cents integer, per api.md's *_cents fields. */
export function money(cents: number): string {
	return `$${(cents / 100).toFixed(2)}`;
}

/** "2h14m" from a start timestamp to now. */
export function uptime(startedAt?: string): string {
	if (!startedAt) return '—';
	const ms = Date.now() - new Date(startedAt).getTime();
	if (ms < 0) return '0m';
	const totalMinutes = Math.floor(ms / 60_000);
	const hours = Math.floor(totalMinutes / 60);
	const minutes = totalMinutes % 60;
	if (hours === 0) return `${minutes}m`;
	return `${hours}h${minutes}m`;
}

const GB = 1024 * 1024 * 1024;

/** "40 GB" from a byte count, the unit every size class is quoted in. */
export function gb(bytes: number): string {
	return `${(bytes / GB).toFixed(bytes % GB === 0 ? 0 : 1)} GB`;
}

/** "2026-09-20 14:02" in the browser's local time, for timestamps that are
 * not "just now" material (created_at, snapshot times). */
export function dateTime(iso: string): string {
	const d = new Date(iso);
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "3 minutes ago" for events and logs. */
export function relativeTime(iso: string): string {
	const ms = Date.now() - new Date(iso).getTime();
	const seconds = Math.floor(ms / 1000);
	if (seconds < 5) return 'just now';
	if (seconds < 60) return `${seconds}s ago`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

/**
 * Projected cost for the whole month at today's run rate, shown beside
 * cost_today/cost_month on the project detail cost card (5.2): what today's
 * hourly rate (cost_today / hours elapsed today) would add up to over every
 * remaining hour this month, on top of what the month has already billed.
 */
export function projectedMonthCents(
	costMonthCents: number,
	costTodayCents: number,
	now = new Date()
): number {
	const hoursElapsedToday = now.getHours() + now.getMinutes() / 60;
	if (hoursElapsedToday < 1) return costMonthCents;
	const ratePerHour = costTodayCents / hoursElapsedToday;
	const totalHoursThisMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate() * 24;
	const hoursElapsedThisMonth = (now.getDate() - 1) * 24 + hoursElapsedToday;
	const hoursRemaining = totalHoursThisMonth - hoursElapsedThisMonth;
	return Math.round(costMonthCents + ratePerHour * hoursRemaining);
}
