package obs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/obs"
	"github.com/heracraft/repose/internal/obs/obslint"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at %s: %v", root, err)
	}
	return root
}

// built reports whether a component's code exists yet. A component is built
// when it has a package tree of its own: internal/hostd and internal/guestd
// exist, internal/api and internal/gateway do not until workstreams 05 and
// 06 land. The rule updates itself, which a hand-kept flag would not.
func built(root string, c obs.Component) bool {
	switch c {
	case obs.ComponentCLI, obs.ComponentAdmin, obs.ComponentHostdev, obs.ComponentHook:
		// These have no required events; see obs.RequiredEvents.
		return true
	}
	_, err := os.Stat(filepath.Join(root, "internal", string(c)))
	return err == nil
}

// TestEventsEmitted is the evidence for the §9 item "every event in §5 is
// emitted by its component: a script greps each event name and lists the
// call site". Run it with -v for the table.
//
// It fails for a component whose code exists and skips the events of one that
// does not, naming them, so that the report says what is missing rather than
// passing quietly.
func TestEventsEmitted(t *testing.T) {
	root := repoRoot(t)
	found, err := obslint.Events(root, []string{"cmd", "internal"})
	if err != nil {
		t.Fatalf("scan the repository for events: %v", err)
	}

	components := make([]obs.Component, 0, len(obs.RequiredEvents))
	for c := range obs.RequiredEvents {
		components = append(components, c)
	}
	sort.Slice(components, func(i, j int) bool { return components[i] < components[j] })

	var pending []string
	for _, c := range components {
		events := obs.RequiredEvents[c]
		if len(events) == 0 {
			continue
		}
		isBuilt := built(root, c)
		t.Logf("--- %s (%d events, %s) ---", c, len(events), map[bool]string{true: "built", false: "not built yet"}[isBuilt])
		for _, ev := range events {
			sites := sitesFor(found[ev], c)
			switch {
			case len(sites) > 0:
				t.Logf("  %-20s %s", ev, strings.Join(sites, ", "))
			case !isBuilt:
				pending = append(pending, string(c)+"/"+ev)
				t.Logf("  %-20s (component not built yet)", ev)
			default:
				t.Errorf("%s must emit %q and no call site does", c, ev)
			}
		}
	}
	if len(pending) > 0 {
		t.Logf("%d event(s) belong to components that do not exist yet: %s",
			len(pending), strings.Join(pending, " "))
	}
}

// TestEventConstantsMatchTheList: a constant that drifts from the event name
// it documents is worse than no constant.
func TestEventConstantsMatchTheList(t *testing.T) {
	for _, e := range obs.AllRequiredEvents() {
		if e != strings.ToLower(e) || strings.ContainsAny(e, " -.") {
			t.Errorf("event %q is not lower_snake", e)
		}
	}
	// Spot-check the names the dashboards and alerts query by string.
	for _, pair := range [][2]string{
		{obs.EventGuestStart, "guest_start"},
		{obs.EventBuildDone, "build_done"},
		{obs.EventFreezeTimeout, "freeze_timeout"},
		{obs.EventAuthFail, "auth_fail"},
		{obs.EventRequest, "request"},
		{obs.EventPartitionDropFail, "partition_drop_fail"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("constant is %q, want %q", pair[0], pair[1])
		}
	}
}

// TestEveryComponentHasAList: a component nobody wrote a list for is a
// component whose logs nobody planned.
func TestEveryComponentHasAList(t *testing.T) {
	for _, c := range obs.Components {
		if _, ok := obs.RequiredEvents[c]; !ok {
			t.Errorf("component %s has no entry in RequiredEvents", c)
		}
	}
}

func sitesFor(sites []obslint.Site, c obs.Component) []string {
	var out []string
	for _, s := range sites {
		if s.Component != string(c) {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d", s.File, s.Line))
	}
	return out
}
