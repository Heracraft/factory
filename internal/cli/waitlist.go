package cli

import "fmt"

// waitlistedMessage is what `repose run` prints when the api puts a first
// project on the capacity waitlist (DECISIONS I-269): the place in the
// queue and where the email goes, from the error's detail. An api that
// sends no detail still sends the whole sentence as its message.
func waitlistedMessage(e *APIError) string {
	pos, _ := e.Detail["position"].(float64)
	if pos < 1 {
		return e.Message
	}
	email, _ := e.Detail["email"].(string)
	if email == "" {
		return fmt.Sprintf("repose is at capacity. You're number %d on the waitlist. Run `repose run` again later.", int(pos))
	}
	return fmt.Sprintf("repose is at capacity. You're number %d on the waitlist; we'll email %s when there's room.", int(pos), email)
}
