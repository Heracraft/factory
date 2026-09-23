package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func TestStatusListsListeningProcesses(t *testing.T) {
	var b bytes.Buffer
	writeListening(&b, []ListeningSignal{
		{Port: 3000, Comm: "next-server (v1", AgeSeconds: 1500, RSSBytes: 96 << 20},
		{Port: 5173, Comm: "node", AgeSeconds: 259200, RSSBytes: 410 << 20},
		{Port: 5432},
	})
	want := `  listening  next-server (v1 :3000 up 25m 96.0 MB
             node :5173 up 3d 410.0 MB
             :5432
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

// End to end through the fake api: `repose status` of a running project
// prints what the newest sample says is listening, with no SSH.
func TestStatusShowsTheGuestsListeners(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()
	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatal(err)
	}
	ps, _ := f.env.Client.ListProjects(ctx)
	fake.SetListening(ps[0].ID, []fakeapi.ListeningSignal{{Port: 5173, Comm: "node", AgeSeconds: 3 * 86400, RSSBytes: 410 << 20}})
	out := &discardWriter{}
	f.env.Out = out
	f.env.TargetFor = func(string) sshTarget { t.Fatal("status made an SSH connection"); return sshTarget{} }
	if err := StatusCmd(ctx, f.env, testSlug); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.buf.String(), "  listening  node :5173 up 3d 410.0 MB") {
		t.Errorf("status:\n%s", out.buf.String())
	}
}
