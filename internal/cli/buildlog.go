package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SSEFrame is one server-sent event.
type SSEFrame struct {
	ID    string
	Event string // "" means the default "message" event
	Data  string
}

// parseSSE reads r as text/event-stream, calling emit for each complete
// frame (docs/interfaces/api.md's build-log route: "id:" = seq, "data:" =
// the line; a bare "event: done" frame ends the stream).
func parseSSE(r io.Reader, emit func(SSEFrame)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cur SSEFrame
	var data []string
	flush := func() {
		if len(data) == 0 && cur.Event == "" && cur.ID == "" {
			return
		}
		cur.Data = strings.Join(data, "\n")
		emit(cur)
		cur = SSEFrame{}
		data = nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "id:"):
			cur.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "event:"):
			cur.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
	return sc.Err()
}

// StreamBuildLog implements 07-cli.md §5.8: GET the SSE log, print each
// line with a dim "nix › " prefix, and return the terminal state from the
// "done" event.
func StreamBuildLog(ctx context.Context, c *Client, projectID, opID string, w io.Writer, sinceSeq int) (state string, lastSeq int, err error) {
	path := fmt.Sprintf("%s/projects/%s/ops/%s/log", c.BaseURL, urlEscape(projectID), urlEscape(opID))
	if sinceSeq > 0 {
		path += fmt.Sprintf("?since=%d", sinceSeq)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", sinceSeq, err
	}
	if c.Tokens != nil {
		tok, err := c.Tokens.AccessToken(ctx, false)
		if err != nil {
			return "", sinceSeq, &notLoggedInError{cause: err}
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", sinceSeq, &unreachableError{cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		apiErr, decodeErr := readResponse(resp, nil)
		if decodeErr != nil {
			return "", sinceSeq, decodeErr
		}
		return "", sinceSeq, apiErr
	}

	lastSeq = sinceSeq
	parseErr := parseSSE(resp.Body, func(f SSEFrame) {
		if f.Event == "done" {
			var d struct {
				State string `json:"state"`
			}
			_ = json.Unmarshal([]byte(f.Data), &d)
			state = d.State
			return
		}
		var line struct {
			Seq  int    `json:"seq"`
			Line string `json:"line"`
		}
		if err := json.Unmarshal([]byte(f.Data), &line); err == nil {
			fmt.Fprintf(w, "nix › %s\n", line.Line)
			lastSeq = line.Seq
		}
	})
	return state, lastSeq, parseErr
}

func urlEscape(s string) string {
	// project and op ids are UUIDv7 strings (docs/interfaces/README.md):
	// no characters that need percent-encoding, so a plain pass-through
	// keeps StreamBuildLog's URL building simple.
	return s
}

var fragmentRefRe = regexp.MustCompile(`fragment\.nix:(\d+)(?::(\d+))?`)

// RenderBuildError implements 07-cli.md §5.8's error block: the message
// (with the internal "fragment.nix" name swapped for the user's own
// fragment file), the marked line with two lines of context when the
// fragment source is available, and exit 10.
func RenderBuildError(w io.Writer, message, localFragmentPath string, fragmentSource []byte) {
	base := filepath.Base(localFragmentPath)
	if base == "" || base == "." {
		base = "repose.nix"
	}
	display := strings.ReplaceAll(message, "fragment.nix", base)
	fmt.Fprintf(w, "error: %s\n", firstLine(display))

	m := fragmentRefRe.FindStringSubmatch(message)
	if m == nil {
		return
	}
	line, _ := strconv.Atoi(m[1])
	col := 0
	if m[2] != "" {
		col, _ = strconv.Atoi(m[2])
	}
	fmt.Fprintf(w, "   at %s:%d", base, line)
	if col > 0 {
		fmt.Fprintf(w, ":%d", col)
	}
	fmt.Fprintln(w)
	if len(fragmentSource) == 0 || line < 1 {
		return
	}
	lines := strings.Split(string(fragmentSource), "\n")
	if line > len(lines) {
		return
	}
	numWidth := len(strconv.Itoa(line))
	if line > 1 {
		fmt.Fprintf(w, "      %*d | %s\n", numWidth, line-1, lines[line-2])
	}
	fmt.Fprintf(w, "      %*d | %s\n", numWidth, line, lines[line-1])
	caretCol := col
	if caretCol <= 0 {
		caretCol = leadingSpaces(lines[line-1]) + 1
	}
	fmt.Fprintf(w, "      %*s | %s^\n", numWidth, "", strings.Repeat(" ", max0(caretCol-1)))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' && r != '\t' {
			break
		}
		n++
	}
	return n
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
