package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// I-198: the laptop's zone reaches tmux's global and per-session
// environment, so a window or agent started after the run is in it.
func TestCarryTZReachesTmux(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if _, err := runSSH(ctx, f.target, "tmux new-session -d -s "+testSlug+" -c ~/"+testSlug, nil); err != nil {
		t.Fatal(err)
	}
	// What a base older than I-198 does on attach from a terminal with no
	// TZ: the session's own entry is removed, shadowing the global one.
	if _, err := runSSH(ctx, f.target, "tmux set-environment -t "+testSlug+" -r TZ", nil); err != nil {
		t.Fatal(err)
	}
	_, o, err := syncCredentialsAndCarry(ctx, f.target, t.TempDir(), f.local, credSyncOptions{}, carryOptions{TZ: "Asia/Tokyo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Failed) != 0 || len(o.Sent) != 1 {
		t.Fatalf("outcome = %+v", o)
	}
	for _, scope := range []string{"-g", "-t " + testSlug} {
		got, err := runSSH(ctx, f.target, "tmux show-environment "+scope+" TZ", nil)
		if err != nil {
			t.Fatalf("show-environment %s: %v", scope, err)
		}
		if strings.TrimSpace(string(got)) != "TZ=Asia/Tokyo" {
			t.Fatalf("tmux %s TZ = %q", scope, got)
		}
	}
	// A window opened now runs in the laptop's zone.
	if _, err := runSSH(ctx, f.target, "tmux new-window -d -t "+testSlug+" -n clock 'date +%Z > ~/zone; sleep 5'", nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		b, err := runSSH(ctx, f.target, "cat ~/zone", nil)
		return err == nil && strings.TrimSpace(string(b)) == "JST"
	})
}

// A zone that is not an IANA name never reaches a shell.
func TestLaptopTZOnlyPassesZoneNames(t *testing.T) {
	for zone, ok := range map[string]bool{
		"Asia/Tokyo": true, "America/Argentina/Buenos_Aires": true, "Etc/GMT+3": true, "UTC": true,
		"": false, "Asia/Tokyo; rm -rf ~": false, "$(id)": false, "../etc": false, "EAT ": false,
	} {
		if got := ianaZone.MatchString(zone); got != ok {
			t.Errorf("ianaZone(%q) = %v, want %v", zone, got, ok)
		}
	}
}

// waitFor polls cond for up to ten seconds.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 10s")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// I-198: a run from another zone moves the project's stored zone, so the
// guest's next start writes the laptop's, and the running guest gets it
// at once.
func TestRunMovesTheProjectToTheLaptopsZone(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	prev := laptopTZ
	laptopTZ = func() string { return "Asia/Tokyo" }
	t.Cleanup(func() { laptopTZ = prev })

	if err := runRun(context.Background(), f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	laptopTZ = func() string { return "Europe/Paris" }
	if err := runRun(context.Background(), f.env, RunOptions{NoAttach: true}, false); err != nil {
		t.Fatalf("second run: %v", err)
	}
	projects, err := f.env.Client.ListProjects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects: %v %v", projects, err)
	}
	if projects[0].TZ == nil || *projects[0].TZ != "Europe/Paris" {
		t.Fatalf("project tz = %v", projects[0].TZ)
	}
	got, err := runSSH(context.Background(), f.target, "tmux show-environment -g TZ", nil)
	if err != nil || strings.TrimSpace(string(got)) != "TZ=Europe/Paris" {
		t.Fatalf("guest tmux TZ = %q %v", got, err)
	}
}

// The attach's helper carries the zone beside the attach and never waits
// for a client that is not there.
func TestSessionHelperCarriesTheZone(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if _, err := runSSH(ctx, f.target, "tmux new-session -d -s "+testSlug, nil); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := runSession(ctx, sessionOptions{Slug: testSlug, Target: f.target.Args, Carry: true, TZ: "Asia/Tokyo"}, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("helper took %s with no client attached", time.Since(start))
	}
	got, err := runSSH(ctx, f.target, "tmux show-environment -g TZ", nil)
	if err != nil || strings.TrimSpace(string(got)) != "TZ=Asia/Tokyo" {
		t.Fatalf("guest tmux TZ = %q %v", got, err)
	}
}
