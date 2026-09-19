package sysdep

import (
	"os/user"
	"strconv"
	"sync"
)

// DevUser is the guest user's name. docs/interfaces/guest-conventions.md
// fixes it, and AGENTS.md forbids a synonym.
const DevUser = "dev"

// DevUID and DevGID are the fallbacks used when the guest's passwd and group
// files cannot be read: dev is uid 1000, and 100 is `users`, the default
// primary group of a NixOS normal user.
const (
	DevUID = 1000
	DevGID = 100
)

var devOnce struct {
	sync.Once
	uid, gid int
}

// DevIdentity resolves the guest user's uid and the gid the hook socket and
// the secrets files are given. It prefers a group literally named `dev`, then
// the dev user's own primary group, then the constants above.
//
// It is resolved rather than hardcoded because the guest base (workstream 02)
// decides whether dev's primary group is `dev` or `users`, and a socket in the
// wrong group is a socket the agent wrappers cannot write to.
func DevIdentity() (uid, gid int) {
	devOnce.Do(func() {
		devOnce.uid, devOnce.gid = DevUID, DevGID
		u, err := user.Lookup(DevUser)
		if err != nil {
			return
		}
		if n, err := strconv.Atoi(u.Uid); err == nil {
			devOnce.uid = n
		}
		if n, err := strconv.Atoi(u.Gid); err == nil {
			devOnce.gid = n
		}
		if g, err := user.LookupGroup(DevUser); err == nil {
			if n, err := strconv.Atoi(g.Gid); err == nil {
				devOnce.gid = n
			}
		}
	})
	return devOnce.uid, devOnce.gid
}
