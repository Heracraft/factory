package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // PATCH /me validates tz without depending on the host's zoneinfo

	"github.com/heracraft/repose/internal/ca/sshca"
)

var (
	nameRe       = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	secretNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	slugStripRe  = regexp.MustCompile(`[^a-z0-9-]+`)
	slugRunRe    = regexp.MustCompile(`-+`)
)

const (
	maxFragmentBytes = 256 << 10
	maxSecretBytes   = 64 << 10
	retentionDays    = 30

	// forceEvalErrorMarker in a fragment makes putConfig answer the exact
	// first canonical eval_failed message from nix-build-contract.md
	// instead of applying, so a consumer can test the error path without a
	// real Nix evaluation.
	forceEvalErrorMarker = "repose-force-eval-error"
)

var reservedSecretNames = map[string]bool{
	"ssh_host_ed25519_key":          true,
	"ssh_host_ed25519_key-cert.pub": true,
	"user_ca.pub":                   true,
}

var classes = map[string]int64{
	"small": 20 << 30,
	"large": 40 << 30,
	"xl":    80 << 30,
}

var catalog = []CatalogItem{
	{ID: "bun", Label: "Bun", Group: "languages", Kind: "package", Description: "Bun JavaScript runtime and package manager"},
	{ID: "nodejs", Label: "Node.js", Group: "languages", Kind: "package", Description: "Node.js LTS", Options: []CatalogOption{
		{ID: "version", Type: "enum", Values: []string{"22", "24"}, Default: "24"},
	}},
	{ID: "python3", Label: "Python 3", Group: "languages", Kind: "package", Description: "CPython 3 with pip"},
	{ID: "postgresql", Label: "PostgreSQL", Group: "databases", Kind: "service", Description: "PostgreSQL server as a user service"},
	{ID: "redis", Label: "Redis", Group: "databases", Kind: "service", Description: "Redis server as a user service"},
	{ID: "chromium", Label: "Chromium", Group: "browsers", Kind: "package", Description: "Headless Chromium for browser automation"},
	{ID: "ripgrep", Label: "ripgrep", Group: "tools", Kind: "package", Description: "Fast recursive grep"},
}

var catalogServices = map[string]bool{"postgresql": true, "redis": true}

var buildLines = []string{"evaluating configuration", "building", "built"}

// Users.

type meView struct {
	User
	TZ        string      `json:"tz"`
	CreatedAt time.Time   `json:"created_at"`
	Billing   billingView `json:"billing"`
	Limits    limitsView  `json:"limits"`
	Notify    notifyView  `json:"notify"`
}

type billingView struct {
	Status           string `json:"status"`
	TrialCreditCents int64  `json:"trial_credit_cents"`
	HasCard          bool   `json:"has_card"`
}

type limitsView struct {
	Projects int `json:"projects"`
	XL       int `json:"xl"`
}

type notifyView struct {
	Email   bool    `json:"email"`
	NtfyURL *string `json:"ntfy_url"`
}

func (f *Fake) meOf(u *userRec) meView {
	v := meView{User: u.User, TZ: u.TZ, CreatedAt: u.CreatedAt,
		Billing: billingView{Status: "trial", TrialCreditCents: 2000},
		Limits:  limitsView{Projects: 3, XL: 1},
		Notify:  notifyView{Email: u.NotifyEmail}}
	if u.NtfyURL != "" {
		url := u.NtfyURL
		v.Notify.NtfyURL = &url
	}
	if f.opts.Billing {
		v.Billing = billingView{Status: "active", HasCard: true}
		v.Limits = limitsView{Projects: 10, XL: 3}
	}
	return v
}

func (f *Fake) getMe(w http.ResponseWriter, r *http.Request) *apiError {
	writeJSON(w, http.StatusOK, f.meOf(userFrom(r)))
	return nil
}

func (f *Fake) patchMe(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		TZ     *string `json:"tz"`
		Notify *struct {
			Email   *bool           `json:"email"`
			NtfyURL json.RawMessage `json:"ntfy_url"`
		} `json:"notify"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	u := userFrom(r)
	if body.TZ != nil {
		if _, err := time.LoadLocation(*body.TZ); err != nil || *body.TZ == "" {
			return invalid("tz: unknown time zone")
		}
		u.TZ = *body.TZ
	}
	if body.Notify != nil {
		if body.Notify.Email != nil {
			u.NotifyEmail = *body.Notify.Email
		}
		if len(body.Notify.NtfyURL) > 0 {
			var url *string
			if err := json.Unmarshal(body.Notify.NtfyURL, &url); err != nil {
				return invalid("notify.ntfy_url: string or null")
			}
			if url == nil {
				u.NtfyURL = ""
			} else if !strings.HasPrefix(*url, "https://") && !strings.HasPrefix(*url, "http://") {
				return invalid("notify.ntfy_url: must be an http(s) URL")
			} else {
				u.NtfyURL = *url
			}
		}
	}
	writeJSON(w, http.StatusOK, f.meOf(u))
	return nil
}

func (f *Fake) deleteMe(w http.ResponseWriter, r *http.Request) *apiError {
	u := userFrom(r)
	u.Cancelling = true
	for _, p := range f.projects {
		if p.owner == u.ID && !p.destroyed && p.State == "running" {
			f.stop(p, true)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelling", "retention_days": retentionDays})
	return nil
}

// notifyUnsubscribe fakes the signed-token check with the token being the
// user id itself: this fixture has no HMAC key to sign against, and no CLI
// or dashboard code ever calls this route (a user's browser does, from an
// email), so a real signature has nothing to prove here.
func (f *Fake) notifyUnsubscribe(w http.ResponseWriter, r *http.Request) *apiError {
	id := r.URL.Query().Get("token")
	u, ok := f.users[id]
	if !ok {
		return errf("invalid", "this unsubscribe link is invalid or has expired")
	}
	u.NotifyEmail = false
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("You have been unsubscribed from repose email notifications.\n"))
	return nil
}

func (f *Fake) notifyTest(w http.ResponseWriter, r *http.Request) *apiError {
	u := userFrom(r)
	res := map[string]string{"email": "error", "ntfy": "error"}
	if u.NotifyEmail {
		res["email"] = "ok"
	}
	if u.NtfyURL != "" {
		res["ntfy"] = "ok"
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// Projects.

func slugOf(name string) string {
	s := slugStripRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(slugRunRe.ReplaceAllString(s, "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// project finds a live project the user owns; anything else is not_found
// so a stranger cannot tell a foreign id from a missing one.
func (f *Fake) project(u *userRec, id string) (*project, *apiError) {
	p, ok := f.projects[id]
	if !ok || p.owner != u.ID || p.destroyed {
		return nil, notFound("project")
	}
	return p, nil
}

// projectAny is project, but a destroyed one is still visible (its last
// snapshot is kept for retentionDays).
func (f *Fake) projectAny(u *userRec, id string) (*project, *apiError) {
	p, ok := f.projects[id]
	if !ok || p.owner != u.ID {
		return nil, notFound("project")
	}
	return p, nil
}

func (f *Fake) userProjects(u *userRec) []*project {
	var out []*project
	for _, p := range f.projects {
		if p.owner == u.ID && !p.destroyed {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (f *Fake) event(p *project, kind, agent, summary string) *Event {
	ev := &Event{ID: f.nextID(), TS: f.now(), Kind: kind, Agent: agent, Summary: summary}
	p.events = append(p.events, ev)
	return ev
}

func (f *Fake) newOp(p *project, kind string) *op {
	o := &op{id: f.nextID(), projectID: p.ID, kind: kind}
	o.State = "done"
	o.LogURL = fmt.Sprintf("%s/v1/projects/%s/ops/%s/log", f.Server.URL, p.ID, o.id)
	f.ops[o.id] = o
	return o
}

func opResult(w http.ResponseWriter, o *op) *apiError {
	writeJSON(w, http.StatusAccepted, map[string]string{"op_id": o.id})
	return nil
}

func (f *Fake) run(p *project) {
	now := f.now()
	p.State = "running"
	p.HostID = hostID
	if p.GuestIP == "" {
		f.ipSeq++
		p.GuestIP = fmt.Sprintf("10.64.4.%d", 10+f.ipSeq)
	}
	p.StartedAt = &now
	p.Signals = &Signals{Agents: []AgentSignal{}, GuestdOK: true}
	f.event(p, "guest.started", "", "guest started on "+hostID)
}

func (f *Fake) snapshot(p *project, reason string) *Snapshot {
	s := &Snapshot{ID: f.nextID(), CreatedAt: f.now(), Bytes: p.DiskUsedBytes, Reason: reason}
	p.snapshots = append(p.snapshots, s)
	at := s.CreatedAt
	p.LastSnapshotAt = &at
	f.event(p, "snapshot.created", "", "snapshot taken ("+reason+")")
	return s
}

func (f *Fake) stop(p *project, snapshot bool) {
	if p.State == "running" && snapshot {
		f.snapshot(p, "stop")
	}
	p.State = "stopped"
	p.StartedAt = nil
	p.Signals = nil
	p.GuestIP = ""
	f.event(p, "guest.stopped", "", "guest stopped")
}

func (f *Fake) create(u *userRec, name, remoteURL, class string) (*project, *apiError) {
	if !nameRe.MatchString(name) {
		return nil, invalid("name: must match [A-Za-z0-9._-]{1,64}")
	}
	if _, ok := classes[class]; !ok {
		return nil, invalid("class: must be one of small, large, xl")
	}
	slug := slugOf(name)
	if slug == "" {
		return nil, invalid("name: yields an empty slug")
	}
	for _, p := range f.userProjects(u) {
		switch {
		case p.Name == name:
			return nil, errf("conflict", "a project named %q exists", name).withDetail(map[string]any{"project_id": p.ID})
		case p.Slug == slug:
			return nil, errf("conflict", "project %q has the same slug %q", p.Name, slug).withDetail(map[string]any{"project_id": p.ID})
		case remoteURL != "" && p.RemoteURL == remoteURL:
			return nil, errf("conflict", "project %q already tracks %s", p.Name, remoteURL).withDetail(map[string]any{"project_id": p.ID})
		}
	}
	now := f.now()
	p := &project{
		Project: Project{
			ID: f.nextID(), Name: name, Slug: slug, RemoteURL: remoteURL, Class: class,
			State: "creating", AgentDefault: "claude", BaseVersion: baseVersion,
			VolumeBytes: classes[class], DiskUsedBytes: 1 << 30, CreatedAt: now,
		},
		owner:     u.ID,
		secrets:   map[string]*SecretMeta{},
		eventKeys: map[string]bool{},
	}
	rev := &Revision{ID: f.nextID(), CreatedAt: now, Status: "applied", BaseVersion: baseVersion, AppliedAt: &now}
	p.revisions = append(p.revisions, rev)
	p.ConfigRevisionID = rev.ID
	f.projects[p.ID] = p
	f.event(p, "project.created", "", "project created as "+class)
	f.run(p)
	return p, nil
}

func (f *Fake) listProjects(w http.ResponseWriter, r *http.Request) *apiError {
	out := []Project{}
	for _, p := range f.userProjects(userFrom(r)) {
		out = append(out, p.Project)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) createProject(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Name      string `json:"name"`
		RemoteURL string `json:"remote_url"`
		Class     string `json:"class"`
		TZ        string `json:"tz"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if body.TZ != "" {
		if _, err := time.LoadLocation(body.TZ); err != nil {
			return invalid("tz: unknown time zone")
		}
	}
	p, e := f.create(userFrom(r), body.Name, body.RemoteURL, body.Class)
	if e != nil {
		return e
	}
	if f.opts.CreateDelay > 0 {
		// Under f.mu already (ServeHTTP); only the goroutine takes it.
		o := f.newOp(p, "create")
		o.State = "running"
		p.State = "creating"
		p.OpID = o.id
		out := p.Project
		// The engine's create op builds first, so a project read back a
		// moment after the create is "building", not "creating"
		// (DECISIONS I-114); the fake shows the same sequence.
		p.State = "building"
		go func() {
			time.Sleep(f.opts.CreateDelay)
			f.mu.Lock()
			defer f.mu.Unlock()
			p.OpID = ""
			f.run(p)
			o.State = "done"
		}()
		writeJSON(w, http.StatusCreated, out)
		return nil
	}
	writeJSON(w, http.StatusCreated, p.Project)
	return nil
}

func (f *Fake) getProject(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, p.Project)
	return nil
}

func (f *Fake) patchProject(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Class           *string `json:"class"`
		HoldBaseUpdates *bool   `json:"hold_base_updates"`
		AgentDefault    *string `json:"agent_default"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	if body.Class != nil && *body.Class != p.Class {
		if _, ok := classes[*body.Class]; !ok {
			return invalid("class: must be one of small, large, xl")
		}
		if p.State != "stopped" {
			return invalid("class: change requires the project to be stopped").withDetail(map[string]any{"state": p.State})
		}
		p.Class = *body.Class
	}
	if body.HoldBaseUpdates != nil {
		p.HoldBaseUpdates = *body.HoldBaseUpdates
	}
	if body.AgentDefault != nil {
		if *body.AgentDefault == "" {
			return invalid("agent_default: must not be empty")
		}
		p.AgentDefault = *body.AgentDefault
	}
	writeJSON(w, http.StatusOK, p.Project)
	return nil
}

func (f *Fake) destroyProject(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	o := f.newOp(p, "destroy")
	if p.State == "running" {
		f.stop(p, false)
	}
	// The real destroy stops, then snapshots the stopped volume (I-165).
	f.snapshot(p, "stop")
	p.State = "destroyed"
	p.HostID = ""
	p.destroyed = true
	p.destroyedAt = f.now()
	p.retained = f.now().Add(retentionDays * 24 * time.Hour)
	for _, s := range p.snapshots {
		if s.ExpiresAt == nil {
			t := p.retained
			s.ExpiresAt = &t
		}
	}
	f.event(p, "project.destroyed", "", "volume deleted, last snapshot kept 30 days")
	// api.md: 202 {op_id, state} (I-156); the fake's ops finish at once.
	writeJSON(w, http.StatusAccepted, map[string]string{"op_id": o.id, "state": o.State})
	return nil
}

func (f *Fake) startProject(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	if p.State == "creating" || p.State == "starting" {
		// What the api answers while the create op is still running.
		return errf("conflict", "%s is already starting", p.Slug)
	}
	o := f.newOp(p, "start")
	restart := p.State == "error" // api.md: a start from error is a restart (I-157)
	if p.State != "running" {
		f.run(p)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": o.id, "restart": restart})
	return nil
}

func (f *Fake) stopProject(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Snapshot *bool `json:"snapshot"`
	}
	if e := decodeBody(r, &body, true); e != nil {
		return e
	}
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	o := f.newOp(p, "stop")
	if p.State != "stopped" {
		f.stop(p, body.Snapshot == nil || *body.Snapshot)
	}
	return opResult(w, o)
}

func (f *Fake) findOp(u *userRec, projectID, opID string) (*project, *op, *apiError) {
	p, e := f.projectAny(u, projectID)
	if e != nil {
		return nil, nil, e
	}
	o, ok := f.ops[opID]
	if !ok || o.projectID != p.ID {
		return nil, nil, notFound("op")
	}
	return p, o, nil
}

func (f *Fake) getOp(w http.ResponseWriter, r *http.Request) *apiError {
	_, o, e := f.findOp(userFrom(r), r.PathValue("id"), r.PathValue("op_id"))
	if e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, o.Op)
	return nil
}

// opLog streams the canned build log as SSE. It runs outside mu (see
// ServeHTTP) and so takes it for the lookup only.
func (f *Fake) opLog(w http.ResponseWriter, r *http.Request) *apiError {
	since := 0
	if s := r.URL.Query().Get("since"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return invalid("since: must be a non-negative integer")
		}
		since = n
	}
	f.mu.Lock()
	_, _, e := f.findOp(userFrom(r), r.PathValue("id"), r.PathValue("op_id"))
	f.mu.Unlock()
	if e != nil {
		return e
	}
	var frames []string
	for i, line := range buildLines {
		seq := i + 1
		if seq <= since {
			continue
		}
		b, err := json.Marshal(map[string]any{"seq": seq, "line": line})
		if err != nil {
			return errf("internal", "encoding log line")
		}
		frames = append(frames, fmt.Sprintf("id: %d\ndata: %s\n\n", seq, b))
	}
	frames = append(frames, "event: done\ndata: {\"state\":\"done\"}\n\n")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher) // httptest's recorder and server both flush; a nil flusher only delays delivery
	for _, fr := range frames {
		if _, err := fmt.Fprint(w, fr); err != nil {
			return nil // the client hung up mid-stream; headers are already out
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	return nil
}

func (f *Fake) resizeProject(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		VolumeBytes int64 `json:"volume_bytes"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	if body.VolumeBytes <= p.VolumeBytes {
		return invalid("volume_bytes: volumes only grow").withDetail(map[string]any{"volume_bytes": p.VolumeBytes})
	}
	o := f.newOp(p, "resize")
	p.VolumeBytes = body.VolumeBytes
	f.event(p, "volume.resized", "", fmt.Sprintf("volume grown to %d bytes", body.VolumeBytes))
	return opResult(w, o)
}

func (f *Fake) projectRoute(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, map[string]string{"host_id": p.HostID, "guest_ip": p.GuestIP, "state": p.State})
	return nil
}

// Config.

func (p *project) revision(id string) *Revision {
	for _, rev := range p.revisions {
		if rev.ID == id {
			return rev
		}
	}
	return nil
}

func (f *Fake) getConfig(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	rev := p.revision(p.ConfigRevisionID)
	if rev == nil {
		return errf("internal", "current revision missing")
	}
	out := map[string]any{"revision_id": rev.ID, "fragment": rev.Fragment, "base_version": rev.BaseVersion}
	if len(rev.Menu) > 0 {
		out["menu"] = rev.Menu
	}
	if rev.AppliedAt != nil {
		out["applied_at"] = rev.AppliedAt
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// renderMenu turns a MenuSelection into the home-manager fragment the
// real api would generate. Ids must come from the catalog.
func renderMenu(menu json.RawMessage) (string, *apiError) {
	var sel struct {
		Packages []string `json:"packages"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal(menu, &sel); err != nil {
		return "", invalid("menu: %v", err)
	}
	known := map[string]bool{}
	for _, c := range catalog {
		known[c.ID] = true
	}
	for _, id := range append(append([]string{}, sel.Packages...), sel.Services...) {
		if !known[id] {
			return "", invalid("menu: %q is not in the catalog", id).withDetail(map[string]any{"id": id})
		}
	}
	var b strings.Builder
	b.WriteString("{ pkgs, ... }:\n{\n")
	if len(sel.Packages) > 0 {
		b.WriteString("  home.packages = with pkgs; [ " + strings.Join(sel.Packages, " ") + " ];\n")
	}
	for _, s := range sel.Services {
		if !catalogServices[s] {
			return "", invalid("menu: %q is not a service", s).withDetail(map[string]any{"id": s})
		}
		b.WriteString("  services." + s + ".enable = true;\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func (f *Fake) putConfig(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Fragment *string         `json:"fragment"`
		Menu     json.RawMessage `json:"menu"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	hasMenu := len(body.Menu) > 0 && string(body.Menu) != "null"
	if (body.Fragment == nil) == !hasMenu {
		return invalid("exactly one of fragment or menu is required")
	}
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	var fragment string
	var menu json.RawMessage
	if body.Fragment != nil {
		if len(*body.Fragment) > maxFragmentBytes {
			return invalid("fragment: larger than 256 KB").withDetail(map[string]any{"max_bytes": maxFragmentBytes})
		}
		fragment = *body.Fragment
	} else {
		rendered, e := renderMenu(body.Menu)
		if e != nil {
			return e
		}
		fragment, menu = rendered, body.Menu
	}
	now := f.now()
	o := f.newOp(p, "config")
	// A canned eval failure, for the dashboard and CLI to exercise the
	// error path (08-dashboard.md §7, nix-build-contract.md "What the user
	// reads" — the exact first canonical message) without a real Nix
	// evaluation. A failed build changes nothing: the previous revision
	// stays applied.
	if strings.Contains(fragment, forceEvalErrorMarker) {
		msg := "config error: syntax error at fragment.nix:1:32, unexpected ';'"
		rev := &Revision{ID: f.nextID(), CreatedAt: now, Status: "failed", Error: msg, Fragment: fragment, Menu: menu, BaseVersion: baseVersion}
		p.revisions = append(p.revisions, rev)
		o.State, o.Error = "error", msg
		f.event(p, "config.failed", "", "revision failed: "+msg)
		writeJSON(w, http.StatusAccepted, map[string]string{"revision_id": rev.ID, "op_id": o.id})
		return nil
	}
	rev := &Revision{ID: f.nextID(), CreatedAt: now, Status: "applied", Fragment: fragment, Menu: menu, BaseVersion: baseVersion, AppliedAt: &now}
	p.revisions = append(p.revisions, rev)
	p.ConfigRevisionID = rev.ID
	f.event(p, "config.applied", "", "revision applied")
	writeJSON(w, http.StatusAccepted, map[string]string{"revision_id": rev.ID, "op_id": o.id})
	return nil
}

func (f *Fake) listRevisions(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	out := []Revision{}
	for _, rev := range p.revisions {
		out = append(out, *rev)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) applyRevision(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	rev := p.revision(r.PathValue("rev"))
	if rev == nil {
		return notFound("revision")
	}
	if rev.Status != "applied" {
		return invalid("revision: only a successful revision can be re-applied").withDetail(map[string]any{"status": rev.Status})
	}
	now := f.now()
	rev.AppliedAt = &now
	p.ConfigRevisionID = rev.ID
	o := f.newOp(p, "apply")
	f.event(p, "config.applied", "", "revision re-applied")
	return opResult(w, o)
}

func (f *Fake) getCatalog(w http.ResponseWriter, r *http.Request) *apiError {
	writeJSON(w, http.StatusOK, catalog)
	return nil
}

// Certificates.

func (f *Fake) issueCert(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		PublicKey  string   `json:"public_key"`
		ProjectIDs []string `json:"project_ids"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if !strings.HasPrefix(body.PublicKey, "ssh-") {
		return invalid("public_key: must be an OpenSSH public key")
	}
	if len(body.ProjectIDs) == 0 {
		return invalid("project_ids: at least one project is required")
	}
	u := userFrom(r)
	for _, id := range body.ProjectIDs {
		if _, e := f.project(u, id); e != nil {
			return e
		}
	}
	f.serial++
	c := &cert{serial: f.serial, owner: u.ID, projectIDs: body.ProjectIDs, expiresAt: f.now().Add(12 * time.Hour)}
	f.certs[c.serial] = c
	line := fmt.Sprintf("ssh-ed25519-cert-v01@openssh.com AAAA...fake serial %d", c.serial)
	if f.opts.CA != nil {
		pub, err := sshca.ParsePublicKey(body.PublicKey)
		if err != nil {
			return invalid("public_key: %v", err)
		}
		signed, err := f.opts.CA.User.SignUser(sshca.UserCert{PublicKey: pub, KeyID: u.ID + ":" + u.Handle, Principals: body.ProjectIDs,
			Serial: c.serial, ValidAfter: f.now().Add(-time.Minute), ValidBefore: c.expiresAt})
		if err != nil {
			return errf("internal", "signing: %v", err)
		}
		line = sshca.Marshal(signed)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"certificate": line,
		"serial":      c.serial,
		"expires_at":  c.expiresAt,
		"gateway":     map[string]any{"host": gatewayHost, "port": 22, "host_ca_pub": f.hostCAPub()},
	})
	return nil
}

// hostCAPub and userCAPub are the CA lines: the test CA's when one is
// configured, placeholders otherwise.
func (f *Fake) hostCAPub() string {
	if f.opts.CA != nil {
		return f.opts.CA.Host.PublicLine()
	}
	return hostCAPub
}

func (f *Fake) userCAPub() string {
	if f.opts.CA != nil {
		return f.opts.CA.User.PublicLine()
	}
	return userCAPub
}

func (f *Fake) revokeCerts(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Serial *uint64 `json:"serial"`
		All    bool    `json:"all"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if (body.Serial == nil) == !body.All {
		return invalid("exactly one of serial or all is required")
	}
	u := userFrom(r)
	revoked := []uint64{}
	revoke := func(c *cert) {
		if !c.revoked {
			c.revoked = true
			f.revoked = append(f.revoked, revocation{serial: c.serial, at: f.now()})
		}
		revoked = append(revoked, c.serial)
	}
	if body.All {
		serials := make([]uint64, 0, len(f.certs))
		for s, c := range f.certs {
			if c.owner == u.ID {
				serials = append(serials, s)
			}
		}
		sort.Slice(serials, func(i, j int) bool { return serials[i] < serials[j] })
		for _, s := range serials {
			revoke(f.certs[s])
		}
	} else {
		c, ok := f.certs[*body.Serial]
		if !ok || c.owner != u.ID {
			return notFound("certificate")
		}
		revoke(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": revoked})
	return nil
}

// Secrets.

func (f *Fake) listSecrets(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	out := []SecretMeta{}
	for _, s := range p.secrets {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) putSecret(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Value string `json:"value"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	name := r.PathValue("name")
	if reservedSecretNames[name] {
		return invalid("name: %q is reserved for the guest's sshd material", name)
	}
	if !secretNameRe.MatchString(name) {
		return invalid("name: must match [A-Z][A-Z0-9_]{0,63}")
	}
	raw, err := base64.StdEncoding.DecodeString(body.Value)
	if err != nil {
		return invalid("value: must be base64")
	}
	if len(raw) > maxSecretBytes {
		return invalid("value: larger than 64 KB decoded").withDetail(map[string]any{"max_bytes": maxSecretBytes})
	}
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	now := f.now()
	if s, ok := p.secrets[name]; ok {
		s.UpdatedAt = now
	} else {
		p.secrets[name] = &SecretMeta{Name: name, CreatedAt: now, UpdatedAt: now}
	}
	f.event(p, "secret.updated", "", "secret "+name+" set")
	writeJSON(w, http.StatusOK, p.secrets[name])
	return nil
}

func (f *Fake) deleteSecret(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	name := r.PathValue("name")
	if _, ok := p.secrets[name]; !ok {
		return notFound("secret")
	}
	delete(p.secrets, name)
	f.event(p, "secret.deleted", "", "secret "+name+" removed")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// Snapshots.

func (f *Fake) listSnapshots(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.projectAny(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	out := []Snapshot{}
	for _, s := range p.snapshots {
		out = append(out, *s)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) createSnapshot(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.project(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	o := f.newOp(p, "snapshot")
	f.snapshot(p, "manual")
	return opResult(w, o)
}

func (f *Fake) restoreSnapshot(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		AsNewProject string `json:"as_new_project"`
	}
	if e := decodeBody(r, &body, true); e != nil {
		return e
	}
	u := userFrom(r)
	p, e := f.projectAny(u, r.PathValue("id"))
	if e != nil {
		return e
	}
	var snap *Snapshot
	for _, s := range p.snapshots {
		if s.ID == r.PathValue("sid") {
			snap = s
		}
	}
	if snap == nil {
		return notFound("snapshot")
	}
	if body.AsNewProject != "" {
		np, e := f.restoreAsNew(u, p, snap, body.AsNewProject)
		if e != nil {
			return e
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"op_id": f.newOp(np, "restore").id, "project_id": np.ID})
		return nil
	}
	if p.State != "stopped" {
		return invalid("restore in place requires the project to be stopped").withDetail(map[string]any{"state": p.State})
	}
	o := f.newOp(p, "restore")
	f.event(p, "volume.restored", "", "volume replaced from snapshot "+snap.ID)
	return opResult(w, o)
}

// restoreAsNew is the as-new restore: a new project from p's class and,
// when no live project has it, p's remote (I-167).
func (f *Fake) restoreAsNew(u *userRec, p *project, snap *Snapshot, name string) (*project, *apiError) {
	remote := p.RemoteURL
	for _, q := range f.userProjects(u) {
		if remote != "" && q.RemoteURL == remote {
			remote = ""
		}
		if q.Slug == slugOf(name) {
			return nil, errf("conflict", "a project named %s already exists; pick another name for the restored one", slugOf(name)).withDetail(map[string]any{"reason": "name_taken", "name": slugOf(name)})
		}
	}
	np, e := f.create(u, name, remote, p.Class)
	if e != nil {
		return nil, e
	}
	f.event(np, "volume.restored", "", "restored from snapshot "+snap.ID+" of "+p.Name)
	return np, nil
}

// restorable is p's newest snapshot that has not expired, or nil.
func (f *Fake) restorable(p *project) *Snapshot {
	var best *Snapshot
	for _, s := range p.snapshots {
		if s.ExpiresAt != nil && !s.ExpiresAt.After(f.now()) {
			continue
		}
		if best == nil || !s.CreatedAt.Before(best.CreatedAt) {
			best = s
		}
	}
	return best
}

// listDestroyed is GET /projects/destroyed (I-167).
func (f *Fake) listDestroyed(w http.ResponseWriter, r *http.Request) *apiError {
	u := userFrom(r)
	live := map[string]bool{}
	for _, p := range f.userProjects(u) {
		live[p.Slug] = true
	}
	out := []DestroyedProject{}
	for _, p := range f.projects {
		if p.owner != u.ID || !p.destroyed {
			continue
		}
		s := f.restorable(p)
		if s == nil {
			continue
		}
		out = append(out, DestroyedProject{ID: p.ID, Name: p.Name, Slug: p.Slug, Class: p.Class, RemoteURL: p.RemoteURL,
			VolumeBytes: p.VolumeBytes, DestroyedAt: p.destroyedAt, NameFree: !live[p.Slug], RestorableUntil: s.ExpiresAt, Snapshot: *s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DestroyedAt.After(out[j].DestroyedAt) })
	writeJSON(w, http.StatusOK, out)
	return nil
}

// restoreByName is POST /projects/restore (I-167).
func (f *Fake) restoreByName(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		Slug       string  `json:"slug"`
		ProjectID  string  `json:"project_id"`
		SnapshotID string  `json:"snapshot_id"`
		Name       *string `json:"name"`
		Start      *bool   `json:"start"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	u := userFrom(r)
	if body.Slug == "" && body.ProjectID == "" && body.SnapshotID == "" {
		return invalid("name the project (slug or project_id) or the snapshot (snapshot_id) to restore")
	}
	var src *project
	var snap *Snapshot
	switch {
	case body.SnapshotID != "":
		for _, p := range f.projects {
			for _, s := range p.snapshots {
				if s.ID == body.SnapshotID && p.owner == u.ID && (s.ExpiresAt == nil || s.ExpiresAt.After(f.now())) {
					src, snap = p, s
				}
			}
		}
		if src == nil || (body.Slug != "" && slugOf(body.Slug) != src.Slug) || (body.ProjectID != "" && body.ProjectID != src.ID) {
			return errf("not_found", "that snapshot does not exist or has expired")
		}
	default:
		var cands []*project
		for _, p := range f.projects {
			if p.owner != u.ID || (body.ProjectID != "" && p.ID != body.ProjectID) || (body.ProjectID == "" && p.Slug != slugOf(body.Slug)) {
				continue
			}
			if !p.destroyed && body.ProjectID == "" {
				cands = []*project{p} // a live project with the name is the one meant
				break
			}
			cands = append(cands, p)
		}
		if len(cands) == 0 {
			return errf("not_found", "you have no project called %s", slugOf(body.Slug))
		}
		for _, p := range cands {
			if s := f.restorable(p); s != nil && (snap == nil || s.CreatedAt.After(snap.CreatedAt)) {
				src, snap = p, s
			}
		}
		if snap == nil {
			return errf("not_found", "%s has no snapshot left to restore", cands[0].Slug).withDetail(map[string]any{"reason": "no_snapshot"})
		}
	}
	name := src.Name
	if body.Name != nil {
		name = *body.Name
	}
	np, e := f.restoreAsNew(u, src, snap, name)
	if e != nil {
		return e
	}
	o := f.newOp(np, "restore")
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": o.id, "project_id": np.ID, "name": np.Name, "slug": np.Slug,
		"snapshot_id": snap.ID, "snapshot_created_at": snap.CreatedAt, "from_project_id": src.ID})
	return nil
}

// Events and logs.

func (f *Fake) listEvents(w http.ResponseWriter, r *http.Request) *apiError {
	p, e := f.projectAny(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	out := []Event{}
	since := r.URL.Query().Get("since")
	var sinceTS time.Time
	sinceIsTime := false
	if since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			sinceTS, sinceIsTime = t, true
		}
	}
	for _, ev := range p.events {
		switch {
		case since == "":
		case sinceIsTime && !ev.TS.After(sinceTS):
			continue
		case !sinceIsTime && ev.ID <= since:
			continue
		}
		out = append(out, *ev)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) projectLogs(w http.ResponseWriter, r *http.Request) *apiError {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "console"
	}
	if kind != "console" && kind != "build" && kind != "ops" {
		return invalid("kind: must be console, build or ops")
	}
	if s := r.URL.Query().Get("since"); s != "" {
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return invalid("since: must be RFC 3339")
		}
	}
	p, e := f.projectAny(userFrom(r), r.PathValue("id"))
	if e != nil {
		return e
	}
	var lines []map[string]any
	ts := f.now()
	switch kind {
	case "console":
		lines = []map[string]any{
			{"ts": ts, "kind": kind, "line": "guestd: hello"},
			{"ts": ts, "kind": kind, "line": "tmux: session " + p.Slug + " ready"},
		}
	case "build":
		for i, l := range buildLines {
			lines = append(lines, map[string]any{"ts": ts, "kind": kind, "seq": i + 1, "line": l})
		}
	case "ops":
		for _, ev := range p.events {
			lines = append(lines, map[string]any{"ts": ev.TS, "kind": kind, "line": ev.Kind + ": " + ev.Summary})
		}
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return nil // the client hung up; headers are already out
		}
	}
	return nil
}

// Usage and billing.

func (f *Fake) getUsage(w http.ResponseWriter, r *http.Request) *apiError {
	for _, k := range []string{"from", "to"} {
		if v := r.URL.Query().Get(k); v != "" {
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return invalid("%s: must be YYYY-MM-DD", k)
			}
		}
	}
	writeJSON(w, http.StatusOK, []UsageRow{})
	return nil
}

func (f *Fake) billingDisabled() *apiError {
	if f.opts.Billing {
		return nil
	}
	return errf("billing_disabled", "billing is not configured")
}

func (f *Fake) billingPortal(w http.ResponseWriter, r *http.Request) *apiError {
	if e := f.billingDisabled(); e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": "https://billing.stripe.com/p/session/fake"})
	return nil
}

func (f *Fake) billingSetup(w http.ResponseWriter, r *http.Request) *apiError {
	if e := f.billingDisabled(); e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, map[string]string{"client_secret": "seti_fake_secret_fake"})
	return nil
}

func (f *Fake) billingInvoices(w http.ResponseWriter, r *http.Request) *apiError {
	if e := f.billingDisabled(); e != nil {
		return e
	}
	writeJSON(w, http.StatusOK, []map[string]any{{
		"id": "in_fake_000001", "amount_cents": 4900, "status": "paid",
		"created_at": f.now().AddDate(0, -1, 0), "hosted_url": "https://invoice.stripe.com/i/fake",
	}})
	return nil
}

// billingWebhook stands in for Stripe's endpoint: it verifies nothing (the
// fake has no webhook secret) and answers what the real route answers, so a
// dashboard or CLI test that pokes it sees the documented shape.
func (f *Fake) billingWebhook(w http.ResponseWriter, r *http.Request) *apiError {
	if e := f.billingDisabled(); e != nil {
		return e
	}
	if r.Header.Get("Stripe-Signature") == "" {
		return errf("invalid", "stripe signature verification failed")
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
	return nil
}

// Internal (gateway).

func (f *Fake) internalRoute(w http.ResponseWriter, r *http.Request) *apiError {
	login := r.URL.Query().Get("login")
	i := strings.LastIndex(login, ".")
	if i <= 0 || i == len(login)-1 {
		return invalid("login: must be <slug>.<handle>")
	}
	slug, handle := login[:i], login[i+1:]
	for _, p := range f.projects {
		if p.destroyed || p.Slug != slug {
			continue
		}
		u, ok := f.users[p.owner]
		if !ok || u.Handle != handle {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"project_id": p.ID, "guest_ip": p.GuestIP, "state": p.State, "principals": []string{p.ID},
		})
		return nil
	}
	return notFound("login")
}

func (f *Fake) internalRevoked(w http.ResponseWriter, r *http.Request) *apiError {
	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return invalid("since: must be RFC 3339")
		}
		since = t
	}
	out := []uint64{}
	for _, rv := range f.revoked {
		if !rv.at.Before(since) {
			out = append(out, rv.serial)
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (f *Fake) internalCA(w http.ResponseWriter, r *http.Request) *apiError {
	writeJSON(w, http.StatusOK, map[string]string{"user_ca_pub": f.userCAPub(), "host_ca_pub": f.hostCAPub()})
	return nil
}

func (f *Fake) internalSessions(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		ProjectID  string `json:"project_id"`
		Event      string `json:"event"`
		CertSerial uint64 `json:"cert_serial"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if body.Event != "opened" && body.Event != "closed" {
		return invalid("event: must be opened or closed")
	}
	p, ok := f.projects[body.ProjectID]
	if !ok || p.destroyed {
		return notFound("project")
	}
	f.sessions = append(f.sessions, SessionReport{ProjectID: body.ProjectID, Event: body.Event, CertSerial: body.CertSerial})
	if p.Signals == nil {
		p.Signals = &Signals{Agents: []AgentSignal{}, GuestdOK: true}
	}
	if body.Event == "opened" {
		p.Signals.SSHSessions++
	} else if p.Signals.SSHSessions > 0 {
		p.Signals.SSHSessions--
	}
	writeJSON(w, http.StatusOK, map[string]int{"ssh_sessions": p.Signals.SSHSessions})
	return nil
}

func (f *Fake) internalHosts(w http.ResponseWriter, r *http.Request) *apiError {
	if f.hosts != nil {
		writeJSON(w, http.StatusOK, f.hosts)
		return nil
	}
	writeJSON(w, http.StatusOK, []Host{{
		HostID: hostID, WGPubkey: "fakewgpubkey0000000000000000000000000000000=", WGIP: "10.64.0.1",
		GuestCIDR: "10.64.4.0/22", State: "ready",
	}})
	return nil
}

func (f *Fake) internalGatewayCerts(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		PublicKey string `json:"public_key"`
		ProjectID string `json:"project_id"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if !strings.HasPrefix(body.PublicKey, "ssh-") {
		return invalid("public_key: must be an OpenSSH public key")
	}
	p, ok := f.projects[body.ProjectID]
	if !ok || p.destroyed {
		return notFound("project")
	}
	f.serial++
	line := fmt.Sprintf("ssh-ed25519-cert-v01@openssh.com AAAA...fake serial %d principal %s key_id %s:via-gateway", f.serial, p.ID, p.ID)
	expires := f.now().Add(sshca.GatewayCertTTL)
	if f.opts.CA != nil {
		pub, err := sshca.ParsePublicKey(body.PublicKey)
		if err != nil {
			return invalid("public_key: %v", err)
		}
		u := f.users[p.owner]
		signed, err := f.opts.CA.User.SignUser(sshca.UserCert{PublicKey: pub, KeyID: u.ID + ":" + u.Handle + ":via-gateway", Principals: []string{p.ID},
			Serial: f.serial, ValidAfter: f.now().Add(-time.Minute), ValidBefore: expires})
		if err != nil {
			return errf("internal", "signing: %v", err)
		}
		line = sshca.Marshal(signed)
	}
	f.gatewayCerts++
	writeJSON(w, http.StatusOK, map[string]any{"certificate": line, "expires_at": expires})
	return nil
}

func (f *Fake) internalEvents(w http.ResponseWriter, r *http.Request) *apiError {
	var body struct {
		SourceIP string `json:"source_ip"`
		Agent    string `json:"agent"`
		Kind     string `json:"kind"`
		Summary  string `json:"summary"`
	}
	if e := decodeBody(r, &body, false); e != nil {
		return e
	}
	if body.SourceIP == "" || body.Kind == "" {
		return invalid("source_ip and kind are required")
	}
	for _, p := range f.projects {
		if p.destroyed || p.GuestIP == "" || p.GuestIP != body.SourceIP {
			continue
		}
		key := body.Agent + "\x00" + body.Kind + "\x00" + f.now().Format(time.RFC3339)
		if p.eventKeys[key] {
			writeJSON(w, http.StatusOK, map[string]any{"deduped": true})
			return nil
		}
		p.eventKeys[key] = true
		ev := f.event(p, body.Kind, body.Agent, body.Summary)
		writeJSON(w, http.StatusOK, map[string]any{"id": ev.ID, "deduped": false})
		return nil
	}
	return notFound("project for source_ip")
}
