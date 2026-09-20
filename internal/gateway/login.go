package gateway

import (
	"errors"
	"regexp"
	"strings"
)

// BadLoginMessage is the banner a malformed login name produces
// (06-gateway-edge.md §5.2).
const BadLoginMessage = "login name must be <project>.<user>"

// ErrBadLogin is returned by ParseLogin for a login that is not
// <project-slug>.<user-handle>.
var ErrBadLogin = errors.New(BadLoginMessage)

// Slugs and handles are [a-z0-9-] (api.md "handle", "slug"); a slug may
// not contain a dot, so the last dot is the only split point.
var loginPartRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// ParseLogin splits <slug>.<handle> on the last dot
// (docs/interfaces/ssh-gateway.md "Names").
func ParseLogin(user string) (slug, handle string, err error) {
	i := strings.LastIndexByte(user, '.')
	if i <= 0 || i == len(user)-1 {
		return "", "", ErrBadLogin
	}
	slug, handle = user[:i], user[i+1:]
	if !loginPartRe.MatchString(slug) || !loginPartRe.MatchString(handle) {
		return "", "", ErrBadLogin
	}
	return slug, handle, nil
}
