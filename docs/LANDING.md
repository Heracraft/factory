# Landing page: rules for visuals and copy

The owner's rules for `apps/web/src/routes/+page.svelte` and
`apps/web/src/lib/components/landing/`, collected from their reviews in
September 2026. Read this before touching the landing page; each rule exists
because a version broke it and was sent back. `DESIGN-LANGUAGE.md` covers the
general house style (serif headings, palette, borders not boxes); this file
covers what the landing page shows and says.

## What we sell

A dev machine in the cloud in one command, that an agent can wreck. Two
ideas, and every visual serves one of them:

1. `repose run` in a checkout turns your laptop's working state into a
   cloud dev machine that feels like your own box.
2. The agent runs there with full permissions; the damage stays on that
   machine, and it comes back.

Not notifications, not answering from your phone, not "check in from
anywhere", not remote control, not always-on, not collaboration, not a
software factory. Don't show or mention them.

## The reference: "Your working state, in one command"

The owner: "that one is beautiful. It's made with anime.js, and the colour
palette, the design, it's beautiful... we should maintain that aesthetic.
It has to be like that." Every other animated visual (the hero, "Break it
and roll it back") matches it: the same light panels with hairline
borders, the same row and chip styling, the same blue accent for what
moves, the same calm anime.js motion (stagger, travel, settle, rest, loop).
Frames of it for reference: record them from the live page before starting.

## Less is more

- Show only what carries the point. A list of fifteen files where two
  matter is noise: the viewer can't find the important ones fast. Show the
  few things that matter (a small group reads instantly, like `.ssh`,
  `passwords.csv`), cut the rest, or mute it hard.
- Transfers are quick: one folder, or at most three items, moving across;
  not every file.

## Show, don't tell

- The picture carries the explanation. A visitor should get the point in two
  seconds without reading a word.
- No captions or labels that explain the picture ("copied by repose run",
  "not sent", "installed here, for Linux", "never reach the machine",
  "outbound", "another project"). Say it with the visual itself: mute,
  strike, cross out, fade, animate, move.
- No hand-holding text and no walls of text. Headline, one short sentence,
  the picture. Explanatory paragraphs go (the pricing notes went).
- A box that holds only small plain text is not a visual. Give it a mark,
  an icon, real content, or remove it.

## Real, and whole, or not at all

- Real apps and real projects, captured from real runs. No invented sample
  apps (`todo-app`), no drawn fake UIs standing in for real ones.
- A partial imitation of a real tool is worse than none. Either show the
  real thing in full (a real capture of Claude Code, a real LazyVim screen)
  or use a different representation (a mark, a diagram).
- Show tools the way people actually use them: Neovim means a real,
  popular config (LazyVim), not bare stock Neovim.
- No fake window chrome (traffic-light dots, close/minimise/maximise) unless
  it adds meaning. A full-screen app needs no frame.
- Every string in a visual is true: from a capture, the docs, or the CLI
  source. Crop, never retype or invent.

## Names a stranger understands

- A visitor doesn't know what `recruiting` or `wira` is, or that it's a
  repose project. Never show a bare internal name as if it explains itself;
  let the picture say what it is (your repo, your app, your machine).
- Don't repeat the same name (`recruiting` … `/home/dev/recruiting`).
- No product jargon without meaning (`large`, a list like
  `large · sudo · docker` after a name).
- "Project" is ambiguous (a production deployment? the repo on your
  laptop?). Avoid the word in visuals, or make its meaning obvious from the
  picture.
- Every cross, arrow or block must show what it prevents or allows; a red
  cross on "another project" means nothing if the viewer can't tell what
  that is.

## The hero (owner's direction, 2026-09-26)

- Same aesthetic as the working-state section, built with anime.js.
- Don't force a laptop drawing on the left; the form factor didn't work.
- The stakes must be obvious (what could be lost), and snapshots must make
  sense without text: the viewer sees a snapshot being taken and the
  machine coming back from it.
- The poisoned thing is "malicious skill", not "npx malicious-skill".
- No chapter labels ("repose run", "The agent wrecks it", "A snapshot puts
  it back"). The three beats must be understood from the motion alone.
- The laptop is full, not empty: several things live on it (your repo, and
  the private rest of your life: SSH keys, tax documents, cute cat photos).
  The repo visibly opens and its contents copy into the cloud machine; the
  private things stay behind.
- The agent is Claude Code's mascot, orange, turning red when it goes
  rogue: it deletes files on the machine, then reaches out along a visible
  path to the internet, picks up something poisoned (a prompt injection, a
  malicious `npx` skill or package, in the spirit of the Shai-Hulud npm
  worm), and that tries to reach your private files on the laptop and is
  stopped at the machine's wall. Then a snapshot puts the machine back.
- Truth limit: the machine's own contents (the checkout, its `.env`, the
  logins copied to it) are reachable by anything running there
  (`apps/web/src/content/docs/secrets.md`, "What an agent on the machine can
  reach"). Show the attack failing against the laptop; never imply the
  machine's own secrets are out of reach. Don't use a real package name for
  the malicious one.

## "Your working state, in one command"

The owner chose the animated version (anime.js): the laptop's commits and
changed files copy across into the cloud machine panel when `repose run`
fires; `node_modules/` stays behind, struck. Keep it animated.

## Copy

- Never write as if the product were Claude-only: "the agent" drives the
  browser, reads the console; not "Claude Code does X".
- No empty phrasing that sounds generated ("tests in a real browser": as
  opposed to what?). Say what the viewer gains.

## The grid cards (owner's notes, 2026-09-26)

- Browser: show the agent's browser-tool log lines and what the browser is
  doing at the same time; the point is that the agent drives the browser
  and reads the console, and you can watch from your laptop and take over
  to nudge it.
- Localhost: keep the tmux green status bar with the forwarded ports and
  the `localhost:5173` address bar; make it obvious that what you see on
  your laptop is forwarded from the cloud machine. The job listings
  (Boeing, RTX) don't fit the page's vibe.
- Editor: the explorer sidebar must be narrow relative to the code.
- "Break it and roll it back": same aesthetic as the working-state section,
  animated with anime.js.
- "Five agents and a full toolchain on first boot" is super clean; leave it.

## Where terminals are allowed

Terminal UIs appear only in the "On every machine" grid and in "Five agents
and a full toolchain on first boot". Everywhere else (hero, "Your working
state", pricing): realistic GUI or clean illustration, no terminal windows,
no CLI output blocks.

## Pricing

The size table and the call to action. No dashboard screenshot, no notes
paragraphs.

## Process

- Judge the assembled page, not a component in isolation: screenshot it at
  1440×900 and 390px, light and dark, and look before calling anything done.
- When motion could help, it's fine to offer an animated and a static
  version for the owner to choose (anime.js is allowed).
