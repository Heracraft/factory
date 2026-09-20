package api

import (
	"encoding/json"
	"time"
)

// User is one account the fake knows. Options.Users maps bearer tokens to
// these; the zero Options has a single canned user.
type User struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	Email       string `json:"email"`
	GitHubLogin string `json:"github_login"`
}

// Project is the documented Project object.
type Project struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Slug             string     `json:"slug"`
	RemoteURL        string     `json:"remote_url"`
	Class            string     `json:"class"`
	State            string     `json:"state"`
	HostID           string     `json:"host_id,omitempty"`
	GuestIP          string     `json:"guest_ip,omitempty"`
	AgentDefault     string     `json:"agent_default"`
	HoldBaseUpdates  bool       `json:"hold_base_updates"`
	BaseVersion      string     `json:"base_version"`
	ConfigRevisionID string     `json:"config_revision_id"`
	VolumeBytes      int64      `json:"volume_bytes"`
	DiskUsedBytes    int64      `json:"disk_used_bytes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	Signals          *Signals   `json:"signals,omitempty"`
	CostTodayCents   int64      `json:"cost_today_cents"`
	CostMonthCents   int64      `json:"cost_month_cents"`
	LastSnapshotAt   *time.Time `json:"last_snapshot_at,omitempty"`
}

// Signals is Project.signals.
type Signals struct {
	SSHSessions int           `json:"ssh_sessions"`
	TmuxClients int           `json:"tmux_clients"`
	Agents      []AgentSignal `json:"agents"`
}

// AgentSignal is one entry of Signals.Agents.
type AgentSignal struct {
	Agent  string `json:"agent"`
	Window string `json:"window"`
	State  string `json:"state"`
}

// Snapshot is one row of GET /projects/:id/snapshots.
type Snapshot struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Bytes     int64     `json:"bytes"`
	Reason    string    `json:"reason"`
}

// Event is one row of GET /projects/:id/events.
type Event struct {
	ID      string    `json:"id"`
	TS      time.Time `json:"ts"`
	Kind    string    `json:"kind"`
	Agent   string    `json:"agent,omitempty"`
	Summary string    `json:"summary"`
}

// SecretMeta is one row of GET /projects/:id/secrets. Values are never
// kept, let alone returned.
type SecretMeta struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Revision is one config revision.
type Revision struct {
	ID          string          `json:"revision_id"`
	CreatedAt   time.Time       `json:"created_at"`
	Status      string          `json:"status"`
	Error       string          `json:"error,omitempty"`
	Fragment    string          `json:"-"`
	Menu        json.RawMessage `json:"-"`
	BaseVersion string          `json:"-"`
	AppliedAt   *time.Time      `json:"-"`
}

// CatalogItem is one row of GET /catalog.
type CatalogItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Op is GET /projects/:id/ops/:op_id.
type Op struct {
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
	LogURL string `json:"log_url,omitempty"`
}

// Host is one row of GET /internal/hosts.
type Host struct {
	HostID    string `json:"host_id"`
	WGPubkey  string `json:"wg_pubkey"`
	WGIP      string `json:"wg_ip"`
	GuestCIDR string `json:"guest_cidr"`
	State     string `json:"state"`
}

// UsageRow is one row of GET /usage.
type UsageRow struct {
	ProjectID  string           `json:"project_id"`
	Day        string           `json:"day"`
	GuestHours map[string]int64 `json:"guest_hours"`
	GBMonths   float64          `json:"gb_months"`
	EgressGB   float64          `json:"egress_gb"`
	CostCents  int64            `json:"cost_cents"`
}

type userRec struct {
	User
	TZ          string
	CreatedAt   time.Time
	NotifyEmail bool
	NtfyURL     string
	Cancelling  bool
}

type project struct {
	Project
	owner     string
	destroyed bool
	retained  time.Time // destroyed projects keep their last snapshot until then
	secrets   map[string]*SecretMeta
	revisions []*Revision
	snapshots []*Snapshot
	events    []*Event
	eventKeys map[string]bool // (agent, kind, ts second) dedupe for /internal/events
}

type op struct {
	Op
	projectID string
	kind      string
}

type cert struct {
	serial     uint64
	owner      string
	projectIDs []string
	expiresAt  time.Time
	revoked    bool
}

type revocation struct {
	serial uint64
	at     time.Time
}
