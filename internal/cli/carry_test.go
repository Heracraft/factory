package cli

import (
	"context"
	"os"
	"path/filepath"
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

// I-298: the Vercel CLI's login is a token for the whole account and stays
// on the laptop. A copy an earlier run made (the guest's file is the
// laptop's, byte for byte) is removed once, with a notice; a login made in
// the guest is kept; the token itself never enters the stream.
func TestSyncCredentialsLeavesVercelsLoginHome(t *testing.T) {
	const token = `{"token":"NEVER-VERCEL-TOKEN"}`
	for _, tc := range []struct{ goos, laptop string }{
		{"darwin", "Library/Application Support/com.vercel.cli/auth.json"},
		{"linux", ".local/share/com.vercel.cli/auth.json"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			t.Setenv("REPOSE_TEST_GOOS", tc.goos)
			f := newSyncFixture(t)
			home := t.TempDir()
			p := filepath.Join(home, tc.laptop)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(token), 0o600); err != nil {
				t.Fatal(err)
			}
			g := filepath.Join(f.guestHome, ".local", "share", "com.vercel.cli", "auth.json")
			var stream strings.Builder
			observePayload = func(script string, tarball []byte) { stream.WriteString(script); stream.Write(tarball) }
			t.Cleanup(func() { observePayload = nil })
			sync := func() ([]string, *carryOutcome) {
				t.Helper()
				copied, o, err := syncCredentialsAndCarry(context.Background(), f.target, home, f.local, credSyncOptions{}, carryOptions{})
				if err != nil {
					t.Fatal(err)
				}
				return copied, o
			}

			// A fresh guest gets no Vercel login.
			copied, o := sync()
			if strings.Join(copied, ",") != "git" || len(o.Warnings) != 0 {
				t.Fatalf("copied = %v, warnings = %v", copied, o.Warnings)
			}
			if fileExists(g) {
				t.Fatal("the Vercel login reached the guest")
			}

			// The copy an earlier CLI made is removed, and said so once.
			if err := os.MkdirAll(filepath.Dir(g), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(g, []byte(token), 0o600); err != nil {
				t.Fatal(err)
			}
			_, o = sync()
			if fileExists(g) {
				t.Fatal("the old copy is still in the guest")
			}
			if len(o.Warnings) != 1 || o.Warnings[0] != retiredCredNotice {
				t.Fatalf("warnings = %q", o.Warnings)
			}
			if _, o = sync(); len(o.Warnings) != 0 {
				t.Fatalf("second run warnings = %q", o.Warnings)
			}

			// A login made in the guest is the user's, and stays.
			if err := os.WriteFile(g, []byte(`{"token":"made-in-the-guest"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, o = sync(); len(o.Warnings) != 0 {
				t.Fatalf("warnings = %q", o.Warnings)
			}
			if b, err := os.ReadFile(g); err != nil || string(b) != `{"token":"made-in-the-guest"}` {
				t.Fatalf("guest login = %q %v", b, err)
			}
			if strings.Contains(stream.String(), "NEVER-VERCEL-TOKEN") {
				t.Fatal("the laptop's Vercel token is in the stream")
			}
		})
	}
}
