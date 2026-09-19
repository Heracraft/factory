package guest

import (
	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/state"
)

func stateCommand(c *hostdv1.Command) state.Command {
	return state.Command{CommandID: c.CommandId, Kind: Kind(c), GuestID: Target(c)}
}

var stateOpen = state.Open

func agentEvent(agent, kind, summary string) *guestdv1.Notify {
	return &guestdv1.Notify{N: &guestdv1.Notify_AgentEvent{AgentEvent: &guestdv1.AgentEvent{Agent: agent, TmuxWindow: agent, Kind: kind, Summary: summary}}}
}

func warning(kind, detail string) *guestdv1.Notify {
	return &guestdv1.Notify{N: &guestdv1.Notify_Warning{Warning: &guestdv1.Warning{Kind: kind, Detail: detail}}}
}

func stateCommandPtr(c *hostdv1.Command) *state.Command {
	sc := stateCommand(c)
	return &sc
}
