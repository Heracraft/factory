package gateway

import (
	"strings"

	"golang.org/x/crypto/ssh"
)

// envAllowed is the list of environment variables an `env` request may
// carry to the guest (06-gateway-edge.md §5.4). sshd's AcceptEnv in the
// guest is the real guard; this keeps noise off the wire.
var envAllowed = map[string]bool{
	"TERM": true, "LANG": true, "COLORTERM": true, "TZ": true,
}

// EnvAllowed reports whether an environment variable name passes the
// filter: the four names above and every LC_*.
func EnvAllowed(name string) bool {
	return envAllowed[name] || strings.HasPrefix(name, "LC_")
}

// envRequest is the payload of an `env` channel request (RFC 4254 §6.4).
type envRequest struct {
	Name  string
	Value string
}

// filterEnv parses an env request payload and reports whether it may be
// forwarded.
func filterEnv(payload []byte) (envRequest, bool) {
	var req envRequest
	if err := ssh.Unmarshal(payload, &req); err != nil {
		return req, false
	}
	return req, EnvAllowed(req.Name)
}
