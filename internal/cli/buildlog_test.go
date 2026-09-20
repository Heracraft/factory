package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSSE(t *testing.T) {
	raw := "id: 1\ndata: {\"seq\":1,\"line\":\"evaluating\"}\n\n" +
		"id: 2\ndata: {\"seq\":2,\"line\":\"building\"}\n\n" +
		"event: done\ndata: {\"state\":\"done\"}\n\n"
	var frames []SSEFrame
	if err := parseSSE(strings.NewReader(raw), func(f SSEFrame) { frames = append(frames, f) }); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3: %+v", len(frames), frames)
	}
	if frames[0].ID != "1" || frames[0].Data != `{"seq":1,"line":"evaluating"}` {
		t.Fatalf("frame 0 = %+v", frames[0])
	}
	if frames[2].Event != "done" {
		t.Fatalf("frame 2 event = %q, want done", frames[2].Event)
	}
}

func TestStreamBuildLogRendersLinesAndDoneState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("id: 1\ndata: {\"seq\":1,\"line\":\"evaluating configuration\"}\n\n"))
		w.Write([]byte("id: 2\ndata: {\"seq\":2,\"line\":\"built\"}\n\n"))
		w.Write([]byte("event: done\ndata: {\"state\":\"done\"}\n\n"))
	}))
	defer srv.Close()

	client := newClient(srv.URL, staticToken("tok"))
	var buf bytes.Buffer
	state, lastSeq, err := StreamBuildLog(context.Background(), client, "p1", "op1", &buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if state != "done" || lastSeq != 2 {
		t.Fatalf("state=%q lastSeq=%d", state, lastSeq)
	}
	out := buf.String()
	if !strings.Contains(out, "nix › evaluating configuration") || !strings.Contains(out, "nix › built") {
		t.Fatalf("output missing rendered lines: %q", out)
	}
}

func TestRenderBuildErrorGolden(t *testing.T) {
	fragment := "{ pkgs, ... }:\n{\n  home.packages = with pkgs; [\n    nodejs_25\n  ];\n}\n"
	msg := "attribute 'nodejs_25' missing at fragment.nix:4:5"
	var buf bytes.Buffer
	RenderBuildError(&buf, msg, "./repose.nix", []byte(fragment))
	want := "error: attribute 'nodejs_25' missing at repose.nix:4:5\n" +
		"   at repose.nix:4:5\n" +
		"      3 |   home.packages = with pkgs; [\n" +
		"      4 |     nodejs_25\n" +
		"        |     ^\n"
	if buf.String() != want {
		t.Fatalf("RenderBuildError mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestRenderBuildErrorWithoutFragmentSource(t *testing.T) {
	var buf bytes.Buffer
	RenderBuildError(&buf, "evaluation exceeded 60 s", "./repose.nix", nil)
	want := "error: evaluation exceeded 60 s\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}
