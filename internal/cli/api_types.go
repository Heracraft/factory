package cli

import (
	"encoding/json"
	"time"
)

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
	// LastError is the api's "code: message" for the op that last failed
	// (the ops engine writes it; cleared by a successful start), and
	// HostUnreachable its flag for a host that stopped answering. Both are
	// what the CLI reads to say why a project is in `error` (I-153);
	// absent from older api builds, which the CLI treats as "no reason".
	LastError       *string `json:"last_error,omitempty"`
	HostUnreachable bool    `json:"host_unreachable,omitempty"`
	// TZ is the zone the guest gets at its next start; the CLI moves it
	// to the laptop's when they differ (I-198). Absent from older apis.
	TZ *string `json:"tz,omitempty"`
}

type Signals struct {
	SSHSessions int           `json:"ssh_sessions"`
	TmuxClients int           `json:"tmux_clients"`
	Docker      int           `json:"docker"`
	Agents      []AgentSignal `json:"agents"`
	// DockerContainers is the name the api actually sends (Docker's
	// "docker" never arrived, so status printed "docker 0" for everyone);
	// both are kept so --json output does not lose a key.
	DockerContainers int `json:"docker_containers,omitempty"`
	// GuestdOK is the newest sample's word on guestd; nil when the api
	// did not say. A running project with false is one `repose start`
	// restarts (I-157).
	GuestdOK *bool `json:"guestd_ok,omitempty"`
	// Listening is the guest's listening processes from the newest sample
	// (I-200); absent from an api or guest older than that.
	Listening []ListeningSignal `json:"listening,omitempty"`
}

// ListeningSignal is one entry of Signals.Listening.
type ListeningSignal struct {
	Port       int    `json:"port"`
	Comm       string `json:"comm,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
	RSSBytes   int64  `json:"rss_bytes,omitempty"`
}

type AgentSignal struct {
	Agent  string `json:"agent"`
	Window string `json:"window"`
	State  string `json:"state"`
}

type Op struct {
	State  string  `json:"state"`
	Error  OpError `json:"error,omitempty"`
	LogURL string  `json:"log_url,omitempty"`
}

// OpError is an op's error as the api stores it: `{code, message}` (the
// hostd Result's error, or the api's own `invalid`), which an older
// reading as a bare string rendered as nothing (DECISIONS I-114). A bare
// string is still accepted.
type OpError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	// Detail is the host's own wording, for operators (I-159); the CLI
	// shows it only under -v.
	Detail any `json:"detail,omitempty"`
}

func (e *OpError) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*e = OpError{}
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*e = OpError{Message: s}
		return nil
	}
	type raw OpError
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	*e = OpError(r)
	return nil
}

// String is the message alone; the code chooses a prefix only where the
// build contract names one (RenderBuildError).
func (e OpError) String() string { return e.Message }

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
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Bytes     int64      `json:"bytes"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
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
	HostID string `json:"host_id"`
	// HostName is the host's name ("host-01"); the api has returned it
	// since M2 and status shows it instead of the id (I-192).
	HostName string `json:"host_name,omitempty"`
	GuestIP  string `json:"guest_ip"`
	State    string `json:"state"`
}

type NotifyTestResult struct {
	Email string `json:"email"`
	Ntfy  string `json:"ntfy"`
}
