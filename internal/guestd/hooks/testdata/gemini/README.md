# gemini has no completion hook

`docs/features/agents.md` records Gemini CLI's mechanism as the tmux
pane-idle heuristic, because the shipped version has no completion hook to
install. There is therefore no payload to map: `hooks.Map("gemini", ...)`
returns `ErrNoEvent` for anything, and `repose-hook` exits 0 without
posting.

What reports a finished Gemini turn is `internal/guestd/sample`'s watcher:
an agent window whose pane has produced no output for 90 seconds and whose
foreground process is still `gemini` becomes one `completed` event saying
"gemini went idle". The tests for that mechanism are
`TestHeuristicCompletionForAHooklessAgent` in
`internal/guestd/sample/watcher_test.go`.

`heuristic.json` below is what a caller would send if a future release of
Gemini CLI did gain a hook: the socket's own shape, which `Map` accepts for
any agent.
