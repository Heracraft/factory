package cli

import (
	"fmt"
	"io"
	"time"
)

// The listening processes `repose status PROJECT` shows for a running
// guest (DECISIONS I-200): which dev servers are still up, for how long
// and at what memory, so a stale one can be found and stopped by hand.
// Nothing is ever stopped for the user. They come from guestd's newest
// sample (Project.signals.listening), so status stays api-only and works
// when the guest cannot be reached (status-and-logs.md).

// compactAge is "12m", "5h" or "3d".
func compactAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// writeListening prints the list under the status lines:
//
//	listening  node :5173 up 3d 410.0 MB
//	           :5432
func writeListening(w io.Writer, ls []ListeningSignal) {
	for i, l := range ls {
		label := "  listening  "
		if i > 0 {
			label = "             "
		}
		s := fmt.Sprintf(":%d", l.Port)
		if l.Comm != "" {
			s = l.Comm + " " + s
			s += fmt.Sprintf(" up %s %s", compactAge(time.Duration(l.AgeSeconds)*time.Second), humanBytes(l.RSSBytes))
		}
		_, _ = fmt.Fprintln(w, label+s)
	}
}
