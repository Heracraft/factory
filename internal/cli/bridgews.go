package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// The websocket framing the bridge's front reads (RFC 6455): just enough
// to see every CDP message between the machine's browser tools and the
// laptop's Chrome, one whole message at a time (bridgecdp.go). No
// extensions: the front removes Sec-WebSocket-Extensions from the
// upgrade, so Chrome never compresses, and a frame with a reserved bit
// set ends the connection.

const (
	wsOpCont  = 0x0
	wsOpText  = 0x1
	wsOpClose = 0x8
)

// wsMaxMessage bounds one message: a full-page screenshot is tens of MB.
const wsMaxMessage = 256 << 20

type wsFrame struct {
	fin     bool
	rsv     byte
	op      byte
	payload []byte // unmasked
	raw     []byte // as it was read
}

func readWSFrame(r *bufio.Reader) (wsFrame, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return wsFrame{}, err
	}
	f := wsFrame{fin: h[0]&0x80 != 0, rsv: h[0] & 0x70, op: h[0] & 0x0f}
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7f)
	raw := append(make([]byte, 0, 14), h[:]...)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return wsFrame{}, err
		}
		raw = append(raw, b[:]...)
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return wsFrame{}, err
		}
		raw = append(raw, b[:]...)
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > wsMaxMessage {
		return wsFrame{}, fmt.Errorf("websocket frame of %d bytes", n)
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return wsFrame{}, err
		}
		raw = append(raw, key[:]...)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return wsFrame{}, err
	}
	f.raw = append(raw, p...)
	if masked {
		for i := range p {
			p[i] ^= key[i%4]
		}
	}
	f.payload = p
	return f, nil
}

// encodeWSFrame is one final frame; masked is the client's side.
func encodeWSFrame(op byte, payload []byte, masked bool) []byte {
	b := make([]byte, 0, len(payload)+14)
	b = append(b, 0x80|op)
	var m byte
	if masked {
		m = 0x80
	}
	switch n := len(payload); {
	case n < 126:
		b = append(b, m|byte(n))
	case n <= 0xffff:
		b = append(b, m|126, byte(n>>8), byte(n))
	default:
		b = append(b, m|127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	if !masked {
		return append(b, payload...)
	}
	var key [4]byte
	_, _ = rand.Read(key[:])
	b = append(b, key[:]...)
	start := len(b)
	b = append(b, payload...)
	for i := range payload {
		b[start+i] ^= key[i%4]
	}
	return b
}

// readWSMessage reads one whole data message. Control frames, which may
// arrive between a message's fragments, go to onControl as they come.
func readWSMessage(r *bufio.Reader, onControl func(wsFrame) error) (op byte, payload, raw []byte, err error) {
	for {
		f, err := readWSFrame(r)
		if err != nil {
			return 0, nil, nil, err
		}
		if f.rsv != 0 {
			return 0, nil, nil, errors.New("websocket frame with a reserved bit set (an extension the bridge did not agree to)")
		}
		if f.op >= 0x8 {
			if err := onControl(f); err != nil {
				return 0, nil, nil, err
			}
			continue
		}
		switch {
		case op == 0 && f.op == wsOpCont:
			return 0, nil, nil, errors.New("websocket continuation with no message")
		case op != 0 && f.op != wsOpCont:
			return 0, nil, nil, errors.New("websocket message inside another")
		case op == 0:
			op = f.op
		}
		payload = append(payload, f.payload...)
		raw = append(raw, f.raw...)
		if len(payload) > wsMaxMessage {
			return 0, nil, nil, fmt.Errorf("websocket message over %d bytes", wsMaxMessage)
		}
		if f.fin {
			return op, payload, raw, nil
		}
	}
}

// lockedWriter keeps whole messages whole when two goroutines write.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) write(b []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.w.Write(b)
	return err
}

// withoutWSExtensions drops Sec-WebSocket-Extensions from a request head,
// so the upgrade agrees to no compression.
func withoutWSExtensions(head []byte) []byte {
	var out bytes.Buffer
	for _, line := range strings.SplitAfter(string(head), "\n") {
		if k, _, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Sec-WebSocket-Extensions") {
			continue
		}
		out.WriteString(line)
	}
	return out.Bytes()
}

// headerValue is one header of a request or response head.
func headerValue(head []byte, name string) string {
	for _, line := range strings.Split(string(head), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// readResponseHead reads a response's status line and headers.
func readResponseHead(br *bufio.Reader) (head []byte, status int, err error) {
	var buf bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, 0, err
		}
		buf.WriteString(line)
		if buf.Len() > 64<<10 {
			return nil, 0, errors.New("response head over 64 KiB")
		}
		if status == 0 {
			f := strings.Fields(line)
			if len(f) < 2 {
				return nil, 0, fmt.Errorf("bad status line %q", strings.TrimSpace(line))
			}
			if _, err := fmt.Sscanf(f[1], "%d", &status); err != nil {
				return nil, 0, fmt.Errorf("bad status line %q", strings.TrimSpace(line))
			}
			continue
		}
		if strings.TrimRight(line, "\r\n") == "" {
			return buf.Bytes(), status, nil
		}
	}
}

// dialWS opens a websocket client connection to a DevTools server's
// path. Chrome's switch holds the upgrade until the user answers its
// dialog, so the wait is ctx's.
func dialWS(ctx context.Context, addr, path string) (net.Conn, *bufio.Reader, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Now()) })
	defer stop()
	var key [16]byte
	_, _ = rand.Read(key[:])
	_, err = fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		path, addr, base64.StdEncoding.EncodeToString(key[:]))
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	br := bufio.NewReader(c)
	_, status, err := readResponseHead(br)
	if err == nil && status != 101 {
		err = fmt.Errorf("the DevTools server answered %d", status)
	}
	if err != nil {
		_ = c.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, err
	}
	return c, br, nil
}
