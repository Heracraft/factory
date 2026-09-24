package cli

// The measurement behind I-236: how late each way of waiting on an op
// learns that a start finished, at a simulated 200 ms round trip (the
// fake api, a start of 2-3 s, 10 runs each). Skipped unless REPOSE_SIM=1:
//
//	REPOSE_SIM=1 go test ./internal/cli -run TestSimOpWaitRTT -v

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

type rttTransport struct{ next http.RoundTripper }

func (t rttTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(100 * time.Millisecond)
	resp, err := t.next.RoundTrip(r)
	time.Sleep(100 * time.Millisecond)
	return resp, err
}

func TestSimOpWaitRTT(t *testing.T) {
	if os.Getenv("REPOSE_SIM") == "" {
		t.Skip("measurement harness")
	}
	for _, mode := range []string{"old-cli+old-api", "new-cli+old-api", "new-cli+new-api"} {
		var lags []float64
		for i := 0; i < 10; i++ {
			d := 2*time.Second + time.Duration(rand.Intn(1000))*time.Millisecond
			fake := fakeapi.New(fakeapi.Options{StartDelay: d, NoLongPoll: mode != "new-cli+new-api"})
			fp, _ := fake.CreateProject("sim", "small")
			fake.SetState(fp.ID, "stopped")
			c := newClient(fake.URL()+"/v1", staticToken("tok"))
			c.HTTP = &http.Client{Transport: rttTransport{http.DefaultTransport}}
			ctx := context.Background()
			p, _ := c.GetProject(ctx, fp.ID)
			sr, err := c.StartProject(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			doneAt := time.Now().Add(d - 100*time.Millisecond) // the POST's answer took half a RTT
			e := &Env{Client: c, Out: io.Discard, ErrOut: io.Discard}
			switch mode {
			case "old-cli+old-api":
				for {
					_, _ = c.GetProject(ctx, p.ID)
					op, _ := c.GetOp(ctx, p.ID, sr.OpID)
					if op.State == "done" {
						break
					}
					time.Sleep(opPollInterval)
				}
				_, _ = c.GetProject(ctx, p.ID) // the read after the op, before the first ssh
			default:
				_, err = waitOpPhased(ctx, e, p, sr.OpID, newProgress(io.Discard, false), true)
				if err != nil {
					t.Fatal(err)
				}
				// the read after the op runs beside the first ssh (I-237)
			}
			lags = append(lags, float64(time.Since(doneAt).Milliseconds()))
			fake.Close()
		}
		sort.Float64s(lags)
		t.Logf("%-18s lag after the op finished until the first ssh can go: median %4.0f ms, min %4.0f, max %4.0f", mode, (lags[4]+lags[5])/2, lags[0], lags[9])
	}
}
