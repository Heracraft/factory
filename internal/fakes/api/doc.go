// Package api is the in-memory fake of docs/interfaces/api.md: an
// httptest.Server that implements every documented route under /v1 with
// deterministic ids, canned users, and a switch to make any route answer
// any documented error code. The CLI and dashboard tests run against it.
//
// Behaviour that matters to a consumer:
//
//   - Every user route wants `Authorization: Bearer <token>`. With a zero
//     Options any non-empty token is the canned user (handle heracraft);
//     with Options.Users only the listed tokens are accepted. The SSE log
//     route also accepts `?access_token=`. Internal routes need no auth.
//   - Operations complete at once: start, stop, resize, snapshot, restore,
//     config PUT and revision apply return `{op_id}` whose op is already
//     `done`, and the state transition has already happened.
//   - Ids are UUIDv7-looking strings drawn from a counter, so a test can
//     predict them; the first id handed out is
//     01900000-0000-7000-8000-000000000001.
//   - Fail, FailNext and Unfail make a route return an error envelope.
//   - Nothing here rate-limits unless Options.RateLimit is set.
//   - A fragment containing "repose-force-eval-error" makes `PUT /config`
//     answer the first canonical eval_failed message from
//     nix-build-contract.md instead of applying: the revision is `failed`,
//     the op ends in `error`, and the project's config is unchanged.
package api
