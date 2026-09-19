package nixbuild

import (
	"regexp"
	"strconv"
	"strings"
)

// Error is a mapped Nix failure, per docs/interfaces/grpc-hostd.md: the
// code, a message whose first line is a summary followed by a blank line
// and the verbatim Nix output (capped at 32 KB), and the fragment line when
// the location parsed.
type Error struct {
	Code         string
	Message      string
	FragmentLine int32
}

func (e *Error) Error() string { return e.Code + ": " + firstLine(e.Message) }

// MessageCap is the verbatim-output cap from the interface doc.
const MessageCap = 32 << 10

var (
	fragLocRe   = regexp.MustCompile(`fragment\.nix:(\d+):(\d+)`)
	errorLineRe = regexp.MustCompile(`(?m)^\s*error: (.*)$`)
	didYouMean  = regexp.MustCompile(`(?m)^\s*(Did you mean .*\?)\s*$`)
	buildingRe  = regexp.MustCompile(`(?m)building '(/nix/store/[^']+\.drv)'`)
	drvFailRe   = regexp.MustCompile(`(?m)(?:Cannot build|builder for) '(/nix/store/[^']+\.drv)'`)
)

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func capVerbatim(stderr string) string {
	if len(stderr) > MessageCap {
		return stderr[len(stderr)-MessageCap:]
	}
	return stderr
}

func compose(summary, stderr string) string {
	return summary + "\n\n" + strings.TrimRight(capVerbatim(stderr), "\n")
}

// lastError returns the text of the final `error:` line and the byte
// offset where it starts.
func lastError(stderr string) (string, int) {
	ms := errorLineRe.FindAllStringSubmatchIndex(stderr, -1)
	if len(ms) == 0 {
		return "", -1
	}
	m := ms[len(ms)-1]
	return strings.TrimSpace(stderr[m[2]:m[3]]), m[0]
}

// fragmentLine finds the location Nix reported for the final error: the
// first fragment.nix mention after the last `error:` line, else the last
// mention anywhere (the trace), else 0.
func fragmentLine(stderr string) (int32, string) {
	_, off := lastError(stderr)
	if off >= 0 {
		if m := fragLocRe.FindStringSubmatch(stderr[off:]); m != nil {
			l, _ := strconv.Atoi(m[1])
			return int32(l), m[1] + ":" + m[2]
		}
	}
	ms := fragLocRe.FindAllStringSubmatch(stderr, -1)
	if len(ms) > 0 {
		m := ms[len(ms)-1]
		l, _ := strconv.Atoi(m[1])
		return int32(l), m[1] + ":" + m[2]
	}
	return 0, ""
}

// MapEvalError turns `nix eval` stderr into an eval_failed Error.
func MapEvalError(stderr string) *Error {
	msg, _ := lastError(stderr)
	if msg == "" {
		msg = "evaluation failed"
	}
	line, loc := fragmentLine(stderr)
	summary := msg
	if loc != "" {
		summary += " at fragment.nix:" + loc
	}
	switch {
	case strings.Contains(msg, "forbidden in restricted mode") && strings.Contains(msg, "URI"):
		summary = "eval-time fetch not allowed"
		if loc != "" {
			summary += " at fragment.nix:" + loc
		}
		summary += "; use pkgs.fetchurl { url = ...; hash = ...; }"
	case strings.Contains(msg, "cannot fetch") && strings.Contains(msg, "pure evaluation mode"):
		summary += "; use pkgs.fetchurl with a hash instead of builtins.fetch*"
	case strings.Contains(msg, "access to absolute path") && strings.Contains(msg, "pure evaluation mode"):
		summary += "; a fragment may only read files it carries"
	case strings.Contains(msg, "does not exist") && strings.Contains(msg, "The option"):
		summary += "; system services come from the menu or `repose config menu`"
	}
	if m := didYouMean.FindStringSubmatch(stderr); m != nil {
		summary += " (" + strings.ToLower(m[1][:1]) + m[1][1:] + ")"
	}
	return &Error{Code: "eval_failed", Message: compose(summary, stderr), FragmentLine: line}
}

// EvalTimeout is the eval_failed Error for a `timeout` exit.
func EvalTimeout(evalS uint32) *Error {
	s := "evaluation exceeded " + strconv.FormatUint(uint64(evalS), 10) + " s"
	return &Error{Code: "eval_failed", Message: s}
}

// LastDerivation names the derivation a build log was last building.
func LastDerivation(log string) string {
	ms := buildingRe.FindAllStringSubmatch(log, -1)
	if len(ms) == 0 {
		return ""
	}
	return drvName(ms[len(ms)-1][1])
}

func drvName(p string) string {
	base := p[strings.LastIndexByte(p, '/')+1:]
	base = strings.TrimSuffix(base, ".drv")
	if i := strings.IndexByte(base, '-'); i >= 0 && i == 32 {
		return base[i+1:]
	}
	return base
}

// MapBuildError turns `nix build` output into a build_failed Error.
func MapBuildError(stderr string) *Error {
	summary := "build failed"
	if m := drvFailRe.FindStringSubmatch(stderr); m != nil {
		summary = "build of " + drvName(m[1]) + " failed"
	} else if msg, _ := lastError(stderr); msg != "" {
		summary = msg
	}
	if strings.Contains(stderr, "Killed") || strings.Contains(stderr, "killed by signal 9") {
		summary += "; build exceeded 16 GB RAM"
	}
	return &Error{Code: "build_failed", Message: compose(summary, stderr)}
}

// BuildTimeout is the build_timeout Error for a `timeout` exit.
func BuildTimeout(buildS uint32, log string) *Error {
	s := "build exceeded " + strconv.FormatUint(uint64(buildS), 10) + " s"
	if d := LastDerivation(log); d != "" {
		s += "; last derivation: " + d
	}
	return &Error{Code: "build_timeout", Message: s}
}
