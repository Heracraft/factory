package cli

import (
	"context"
	"fmt"
)

// setNtfyURL builds the **string PatchMeRequest.Notify.NtfyURL needs to
// tell "not touching it" (nil outer pointer, omitted) from "clearing it"
// (non-nil outer pointer to a nil inner one, marshals to JSON null) from
// "setting it" (both non-nil).
func setNtfyURL(v string) **string {
	if v == "none" {
		var inner *string
		return &inner
	}
	inner := &v
	return &inner
}

// NotifySetCmd implements `repose notify set --email on|off --ntfy
// <url>|none` (I-8). email/ntfy are nil when that flag was not given.
func NotifySetCmd(ctx context.Context, e *Env, email *bool, ntfy *string) error {
	patch := PatchMeRequest{Notify: &NotifyPatch{}}
	if email != nil {
		patch.Notify.Email = email
	}
	if ntfy != nil {
		patch.Notify.NtfyURL = setNtfyURL(*ntfy)
	}
	me, err := e.Client.PatchMe(ctx, patch)
	if err != nil {
		return err
	}
	ntfyURL := "none"
	if me.Notify.NtfyURL != nil {
		ntfyURL = *me.Notify.NtfyURL
	}
	_, _ = fmt.Fprintf(e.Out, "email: %s\nntfy: %s\n", onOff(me.Notify.Email), ntfyURL)
	return nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// NotifyTestCmd implements `repose notify test`.
func NotifyTestCmd(ctx context.Context, e *Env) error {
	res, err := e.Client.NotifyTest(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "email: %s\nntfy: %s\n", res.Email, res.Ntfy)
	if res.Email != "ok" && res.Ntfy != "ok" {
		return silent(ExitGeneric)
	}
	return nil
}
