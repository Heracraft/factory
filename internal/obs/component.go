package obs

import "fmt"

// Component is the value of the `component` field on every log line and the
// job a metrics endpoint belongs to. docs/workstreams/10-observability.md §5
// names six; two binaries that the same design added later have their own
// (DECISIONS I-49) because a Loki query that cannot separate hostd from the
// stand-in it talks to is not worth running.
type Component string

const (
	ComponentAPI     Component = "api"
	ComponentHostd   Component = "hostd"
	ComponentGuestd  Component = "guestd"
	ComponentGateway Component = "gateway"
	ComponentCLI     Component = "cli"
	ComponentAdmin   Component = "admin"

	// ComponentHostdev is the one-host api stand-in of DECISIONS I-17.
	ComponentHostdev Component = "hostdev"
	// ComponentHook is repose-hook, the in-guest helper that posts an agent
	// event to guestd's hook socket.
	ComponentHook Component = "hook"
)

// Components is every valid component, in the order §5 lists them.
var Components = []Component{
	ComponentAPI, ComponentHostd, ComponentGuestd, ComponentGateway,
	ComponentCLI, ComponentAdmin, ComponentHostdev, ComponentHook,
}

// Valid reports whether c is one of Components.
func (c Component) Valid() bool {
	for _, k := range Components {
		if c == k {
			return true
		}
	}
	return false
}

func (c Component) String() string { return string(c) }

// MetricsPort is the port the component serves /metrics on, from
// docs/ops/OBSERVABILITY.md "Where things are". Zero means the component
// does not serve metrics: the CLI ships nothing from a laptop, and
// repose-hook lives for milliseconds.
func (c Component) MetricsPort() int {
	switch c {
	case ComponentHostd, ComponentHostdev:
		return 9101
	case ComponentGateway:
		return 9102
	case ComponentAPI:
		return 9103
	default:
		return 0
	}
}

func (c Component) check() error {
	if !c.Valid() {
		return fmt.Errorf("obs: unknown component %q; use one of %v", c, Components)
	}
	return nil
}
