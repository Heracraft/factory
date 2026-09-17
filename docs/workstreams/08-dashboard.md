# 08 · dashboard

## 1. Goal

The web dashboard at `factory.herakraft.co` is where users do the things that
are awkward in a terminal: pick packages from a menu without writing Nix,
manage secrets, look at cost, put a card on file, and see what an agent did
overnight. It shows nothing the CLI cannot also do, and it does nothing the
API does not do for it.

## 2. Scope: builds

- `apps/web` (existing SvelteKit 5 app, Svelte 5 runes, Vite, adapter-node).
- Logto login as a single-page app (authorization code with PKCE in the
  browser via `@logto/browser`), access tokens for the API resource, refresh
  in the client. No server-side session, no server-side secrets.
- Pages: sign-in, projects list, project detail (status, signals, events,
  cost, snapshots), config (menu and raw fragment editor with build log and
  errors), secrets, billing (card setup, portal link, invoices, usage),
  settings (timezone, notification channels), account (cancel).
- A thin typed API client generated from `interfaces/api.md` shapes in
  `apps/web/src/lib/api/`.
- Dockerfile with a `HEALTHCHECK` and a `/healthz` route so Coolify does a
  rolling deploy.
- Landing page at `/` for signed-out visitors: what it is, pricing, install
  command.
- Playwright tests against `internal/fakes/api` served over HTTP.

## 3. Scope: does not build

- The API, the catalog contents, menu-to-fragment rendering (all
  05-control-plane-api; the dashboard sends `{menu}` and shows what comes
  back).
- Web terminal to the guest (DECISIONS R4-18, not built).
- Preview URLs (DESIGN §7, later).
- Teams, org switching (R5-6).
- Stripe Elements beyond the SetupIntent card form (09-billing owns the
  Stripe side; the dashboard embeds the card form and links to the portal).
- Admin or operator views (`factory-admin`, 05).

## 4. Interfaces

Owns: none.

Consumes: `interfaces/api.md` (all user routes), `interfaces/cli-config.md`
(only to show the same project slugs and names the CLI shows).

## 5. Design detail

### 5.1 Auth

`@logto/browser` `LogtoClient` with `appId` for the dashboard's SPA app,
`resources: ["https://api.factory.herakraft.co"]`, `scopes: ["openid",
"profile", "email", "offline_access"]`. `+layout.ts` checks
`isAuthenticated()`; unauthenticated users see the landing page and a "Sign
in with GitHub" button that calls `signIn(callbackUrl)`. `/callback` handles
`handleSignInCallback`. Every API call does `getAccessToken(resource)` and
sends it as a bearer. Tokens live in memory plus Logto's default storage
(localStorage) for the refresh token; this is the accepted SPA pattern and
the API's audience check is the control.

The dashboard has no `+server.ts` routes except `/healthz`. Nothing in
`apps/web` reads an environment secret; the only build-time env values are
`PUBLIC_API_URL`, `PUBLIC_LOGTO_ENDPOINT`, `PUBLIC_LOGTO_APP_ID`,
`PUBLIC_STRIPE_PUBLISHABLE_KEY`.

### 5.2 Routes

| Route | Content |
|---|---|
| `/` | landing (signed out) or redirect to `/projects` (signed in) |
| `/callback` | Logto callback |
| `/projects` | table: name, class, state (dot + word), uptime, agent state, cost today, cost month. Row click → detail. `New project` explains that projects are created from the CLI and shows the install command; there is no create form because a project needs a git remote and a laptop-side sync. |
| `/projects/[id]` | header with state and actions (Start, Stop, Destroy with confirm typing the slug); cards: connect (`factory run` and `ssh <slug>.factory`), signals (ssh sessions, tmux clients, agents and their state, docker containers, updated N s ago), cost (today, month, projected month at current run rate, using `GET /usage`), disk (used / allocated, Resize with a size picker), events (list from `GET /events`, newest first, agent icon, summary), snapshots (list, Create, Restore with confirm, restore-as-new with a name field), last build (status, link to config) |
| `/projects/[id]/config` | two tabs: **Menu** and **Nix**. Menu: groups from `GET /catalog` rendered as checkbox lists with descriptions and a search box, plus a "Services" group for things like Postgres and Redis if the catalog has them; Apply sends `{menu}`. Nix: CodeMirror 6 editor with Nix syntax, Apply sends `{fragment}`. Both then open the build log panel (SSE from `/ops/:op/log`), auto-scrolled, and on failure show the error block with the fragment line highlighted in the editor. Revisions list with Re-apply. A `Hold base updates` toggle (PATCH `hold_base_updates`) with the current base version and its changelog. |
| `/projects/[id]/secrets` | list of names with dates; Add (name, value textarea or file upload, client validates the name regex); Delete with confirm. Values are never displayed after save. |
| `/billing` | status banner (trial credit left, past due, suspended); card on file (Stripe Elements `PaymentElement` in setup mode using `POST /billing/setup`); "Manage in Stripe" (`POST /billing/portal` → redirect); invoices table; usage chart for the month by project (bar per day, stacked by class) from `GET /usage`. |
| `/settings` | timezone (auto-detected default, select), email notifications toggle, ntfy URL field with a "Send test" button (calls `POST /me/notify-test`, added to `interfaces/api.md` by this workstream if missing: see §6), install command, SSH config hint. |
| `/account` | handle, email, GitHub login, Delete account (types handle, calls `DELETE /me`, explains 30-day retention). |
| `/healthz` | `200 ok` |

### 5.3 State and polling

Svelte 5 runes stores in `src/lib/state/`. The projects list and project
detail poll `GET /projects` / `GET /projects/:id` every 10 seconds while the
tab is visible (`document.visibilityState`), and every 60 seconds otherwise.
Ops (start, stop, build) poll `GET /ops/:op` every 2 seconds until done.
Build logs use `EventSource` with the bearer token passed as
`?access_token=` (the API accepts it on the SSE route only; recorded in
`interfaces/api.md` by 05).

### 5.4 Menu → fragment

The dashboard never renders Nix from the menu. It sends `{menu:
MenuSelection}` where `MenuSelection = {packages: [catalog id], services:
[catalog id], options: {[catalog id]: value}}`, and displays the fragment the
API stored (`GET /config` returns both). Switching from Menu to Nix tab after
a menu apply shows the generated fragment read-only with an "Edit as Nix"
button that copies it into the editor and marks the project as
hand-edited (the API sets `menu = null` on a fragment apply, and the Menu tab
then shows "This project's config was edited by hand; applying from the menu
will replace it").

### 5.5 Errors

Every API error renders as a toast with `message`; `payment_required` on
Start renders an inline banner linking to `/billing`; `capacity` renders
"No capacity right now, try again in a few minutes"; `rate_limited` waits and
retries once. Network failures show a persistent "Cannot reach the API" bar
until a poll succeeds.

### 5.6 Deploy

`apps/web/Dockerfile`: multi-stage, `node:24-alpine`, `pnpm install
--frozen-lockfile`, `pnpm --filter web build`, runtime image runs `node
build` on port 3000 as a non-root user, `HEALTHCHECK CMD wget -qO-
http://127.0.0.1:3000/healthz || exit 1`. Coolify application type "Dockerfile",
health check path `/healthz`, no host port mapping (Traefik routes
`factory.herakraft.co` → 3000), so deploys are rolling. Build args carry
the four `PUBLIC_*` values.

### 5.7 Landing page

One screen: the one-idea sentence, the four-line terminal example from
`DESIGN.md` §2, pricing table from `PRICING.md` (small/large/xl caps,
storage, egress, trial), install command, "Sign in with GitHub". Links to
terms and privacy (static markdown rendered from `docs/legal/` once 14
writes them; placeholders until then must be visibly marked draft).

## 6. Failure modes

| Situation | Outcome |
|---|---|
| Logto sign-in fails or is cancelled | back to landing with a toast `Sign-in was cancelled or failed; try again.` |
| Access token refresh fails | sign out, redirect to landing, toast `Session expired, sign in again.` |
| API 5xx or unreachable | persistent bar, polling continues with backoff to 60 s |
| Build fails | error block under the editor, fragment line highlighted, revision marked failed, Apply re-enabled |
| Start with no card | inline banner with a link to `/billing`; button stays enabled |
| Destroy typed wrong | button disabled until the slug matches exactly |
| Secret value over 64 KB | client-side error before sending |
| `POST /me/notify-test` missing on the API | button shows `Test not available yet`; this workstream files the route in `interfaces/api.md` and DECISIONS I-n |
| `/healthz` fails in the container | Coolify does not switch traffic; old container keeps serving |

## 7. Testing

- Unit (Vitest): API client error mapping, menu selection state, remote
  normalisation display, cost projection math.
- Component tests for the config editor error highlighting and the destroy
  confirm.
- Playwright end to end against `internal/fakes/api` (Go, run as a test
  fixture on a port) with a fake Logto (a tiny OIDC stub in
  `test/fake-logto/` that issues signed tokens the fake API accepts): sign
  in, see projects, open detail, start and stop, apply menu, apply broken
  fragment and see the error, add and delete a secret, set ntfy URL.
- Docker build in CI and a `HEALTHCHECK` probe.
- Real: after M3, walk every page against production with a real account
  and record it in `STATUS.md`.

## 8. Rollback

Coolify keeps previous images; redeploy the previous build. The dashboard
holds no state, so there is nothing to migrate. If a new dashboard depends on
an API route the API does not have yet, the page must degrade (see the
notify-test row), never break the whole app.

## 9. Checklist

- [ ] Every route in 5.2 exists and renders with the fake API. Evidence:
      Playwright run listing each route.
- [ ] Sign-in, callback, token refresh and sign-out work against the real
      Logto with the GitHub connector. Evidence: recording or transcript.
- [ ] No server routes other than `/healthz`; no `$env/static/private` or
      `$env/dynamic/private` imports anywhere. Evidence: `rg 'env/static/private|env/dynamic/private|\+server\.ts' apps/web/src` shows only `healthz`.
- [ ] Projects list and detail poll at 10 s visible / 60 s hidden and stop
      when the tab is closed. Evidence: network log screenshot or test.
- [ ] Start, Stop, Destroy, Resize call the right routes and show op
      progress. Evidence: Playwright test.
- [ ] Menu tab renders every catalog group and search filters it; Apply
      sends `{menu}` and shows the generated fragment. Evidence: Playwright
      test with a fixture catalog.
- [ ] Nix tab: editor with Nix highlighting, Apply, build log streams, a
      failing fragment shows the error block with the line highlighted.
      Evidence: Playwright test with the fake API's canned eval error.
- [ ] Hold base updates toggle round-trips. Evidence: test.
- [ ] Secrets: add via text and via file, list, delete; values never appear
      in the DOM after save. Evidence: test asserts on DOM.
- [ ] Billing: SetupIntent card form saves a Stripe test card; portal link
      redirects; invoices and usage render from fixtures. Evidence: test
      plus a screenshot against Stripe test mode.
- [ ] Settings: timezone, email toggle, ntfy URL, test button. Evidence:
      test.
- [ ] Account deletion flow requires typing the handle and explains
      retention. Evidence: test.
- [ ] Every failure row in §6 is exercised. Evidence: Playwright tests
      named after the rows.
- [ ] Dockerfile builds in CI, runs as non-root, `HEALTHCHECK` passes, image
      under 200 MB. Evidence: CI log with image size.
- [ ] Coolify deploy is rolling: deploy twice while `curl` loops against
      the domain, zero non-200 responses. Evidence: the curl loop output.
- [ ] Landing page contains the install command, pricing table matching
      `PRICING.md`, and links to terms and privacy. Evidence: screenshot.
- [ ] Lighthouse accessibility score 90 or higher on `/projects` and
      `/projects/[id]/config`. Evidence: report.
- [ ] `features/config.md`, `features/secrets.md`, `features/snapshots.md`
      match what the pages do. Evidence: implementer re-read.
- [ ] `ops/RUNBOOK.md` has: dashboard up but API bar showing, sign-in loop.
      Evidence: entries exist.
