// Shapes copied from docs/interfaces/api.md. This file is the thin typed
// client's contract; a field added there without a matching field here is a
// bug in this file, not a reason to read the response as `any`.

export type SizeClass = 'small' | 'large' | 'xl';

export type GuestState =
	| 'creating'
	| 'building'
	| 'starting'
	| 'running'
	| 'stopping'
	| 'stopped'
	| 'restoring'
	| 'destroying'
	| 'destroyed'
	| 'error';

export type BillingStatus = 'trial' | 'active' | 'past_due' | 'suspended' | 'exempt';

export type ErrorCode =
	| 'unauthenticated'
	| 'forbidden'
	| 'not_found'
	| 'invalid'
	| 'conflict'
	| 'payment_required'
	| 'capacity'
	| 'rate_limited'
	| 'internal'
	| 'billing_disabled';

export interface ApiErrorBody {
	code: ErrorCode;
	message: string;
	detail?: Record<string, unknown>;
}

export interface Me {
	id: string;
	handle: string;
	email: string;
	github_login: string;
	tz: string;
	created_at: string;
	billing: {
		status: BillingStatus;
		trial_credit_cents: number;
		has_card: boolean;
	};
	limits: {
		projects: number;
		xl: number;
	};
	notify?: {
		email: boolean;
		ntfy_url: string | null;
	};
}

export interface AgentSignal {
	agent: string;
	window: string;
	state: string;
}

export interface Signals {
	ssh_sessions: number;
	tmux_clients: number;
	agents: AgentSignal[];
	/** False when the newest sample found the environment's agent not answering (I-157). */
	guestd_ok?: boolean;
}

export interface Project {
	id: string;
	name: string;
	slug: string;
	remote_url: string;
	class: SizeClass;
	state: GuestState;
	host_id?: string;
	guest_ip?: string;
	agent_default: string;
	hold_base_updates: boolean;
	base_version: string;
	config_revision_id: string;
	volume_bytes: number;
	disk_used_bytes?: number;
	created_at: string;
	started_at?: string;
	signals?: Signals;
	cost_today_cents: number;
	cost_month_cents: number;
	last_snapshot_at?: string;
	/** The last failed op's "code: sentence" (I-159); null once an op succeeds. */
	last_error?: string | null;
	host_unreachable?: boolean;
}

/** GET /projects/destroyed (I-167): a destroyed project that can still be restored. */
export interface DestroyedProject {
	id: string;
	name: string;
	slug: string;
	class: SizeClass;
	remote_url?: string | null;
	volume_bytes: number;
	destroyed_at: string;
	/** Whether a restore can take the old name (no live project holds it). */
	name_free: boolean;
	restorable_until?: string | null;
	snapshot: Snapshot;
}

/** POST /projects/restore's answer (I-167). */
export interface RestoreResult {
	op_id: string;
	project_id: string;
	name: string;
	slug: string;
	snapshot_id: string;
	snapshot_created_at: string;
	from_project_id: string;
}

export interface OpStatus {
	state: 'pending' | 'running' | 'done' | 'error';
	error?: string;
	log_url?: string;
}

export interface MenuSelection {
	packages: string[];
	services: string[];
	options: Record<string, string>;
}

export interface Config {
	revision_id: string;
	fragment: string;
	menu?: MenuSelection | null;
	base_version: string;
	applied_at?: string;
}

export interface Revision {
	revision_id: string;
	created_at: string;
	status: 'building' | 'applied' | 'failed';
	error?: string;
}

export interface CatalogOption {
	id: string;
	type: string;
	values: string[];
	default: string;
}

export interface CatalogItem {
	id: string;
	label: string;
	group: string;
	kind: 'service' | 'package' | 'agent' | 'runtime';
	description: string;
	options?: CatalogOption[];
}

export interface SecretMeta {
	name: string;
	created_at: string;
	updated_at: string;
}

export interface Snapshot {
	id: string;
	created_at: string;
	bytes: number;
	reason: string;
	expires_at?: string | null;
}

export interface ProjectEvent {
	id: string;
	ts: string;
	kind: string;
	agent?: string;
	summary: string;
}

export interface UsageRow {
	guest_hours: Partial<Record<SizeClass, number>>;
	gb_months: number;
	egress_gb: number;
	cost_cents: number;
}

export interface Route {
	host_id: string;
	guest_ip: string;
	state: GuestState;
}
