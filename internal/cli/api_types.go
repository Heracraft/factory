package cli

import "time"

// These mirror docs/interfaces/api.md exactly (field names and JSON tags
// match internal/fakes/api's Project et al. so the CLI decodes the fake
// and the real api identically).

type Me struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	Email       string    `json:"email"`
	GitHubLogin string    `json:"github_login"`
	TZ          string    `json:"tz"`
	CreatedAt   time.Time `json:"created_at"`
	Billing     struct {
		Status           string `json:"status"`
		TrialCreditCents int64  `json:"trial_credit_cents"`
		HasCard          bool   `json:"has_card"`
	} `json:"billing"`
	Limits struct {
		Projects int `json:"projects"`
		XL       int `json:"xl"`
	} `json:"limits"`
	Notify struct {
		Email   bool    `json:"email"`
		NtfyURL *string `json:"ntfy_url"`
	} `json:"notify"`
}

type Project struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Slug             string     `json:"slug"`
	RemoteURL        string     `json:"remote_url"`
	Class            string     `json:"class"`
	State            string     `json:"state"`
	OpID             string     `json:"op_id,omitempty"` // the op a create or start left in flight
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

type Signals struct {
	SSHSessions int           `json:"ssh_sessions"`
	TmuxClients int           `json:"tmux_clients"`
	Docker      int           `json:"docker"`
	Agents      []AgentSignal `json:"agents"`
}

type AgentSignal struct {
	Agent  string `json:"agent"`
	Window string `json:"window"`
	State  string `json:"state"`
}

type Op struct {
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
	LogURL string `json:"log_url,omitempty"`
}

type CertResponse struct {
	Certificate string    `json:"certificate"`
	ExpiresAt   time.Time `json:"expires_at"`
	Gateway     struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		HostCAPub string `json:"host_ca_pub"`
	} `json:"gateway"`
}

type SecretMeta struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Revision struct {
	ID             string    `json:"revision_id"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	FragmentLine   *int      `json:"fragment_line,omitempty"`
	KernelChanged  bool      `json:"kernel_changed,omitempty"`
	RebootRequired bool      `json:"reboot_required,omitempty"`
}

type ConfigResponse struct {
	RevisionID  string     `json:"revision_id"`
	Fragment    string     `json:"fragment"`
	Menu        any        `json:"menu,omitempty"`
	BaseVersion string     `json:"base_version"`
	AppliedAt   *time.Time `json:"applied_at,omitempty"`
}

type Snapshot struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Bytes     int64     `json:"bytes"`
	Reason    string    `json:"reason"`
}

type Event struct {
	ID      string    `json:"id"`
	TS      time.Time `json:"ts"`
	Kind    string    `json:"kind"`
	Agent   string    `json:"agent,omitempty"`
	Summary string    `json:"summary"`
}

type CatalogItem struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Group       string          `json:"group"`
	Kind        string          `json:"kind"`
	Description string          `json:"description"`
	Options     []CatalogOption `json:"options,omitempty"`
}

type CatalogOption struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

type Route struct {
	HostID  string `json:"host_id"`
	GuestIP string `json:"guest_ip"`
	State   string `json:"state"`
}

type NotifyTestResult struct {
	Email string `json:"email"`
	Ntfy  string `json:"ntfy"`
}
