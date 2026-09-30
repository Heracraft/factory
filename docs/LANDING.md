# Landing page: rules for visuals and copy

The owner's rules for `apps/web/src/routes/+page.svelte` and
`apps/web/src/lib/components/landing/`, collected from their reviews in
September 2026. Read this before touching the landing page; each rule exists
because a version broke it and was sent back. `DESIGN-LANGUAGE.md`,
"Foundation", holds what the landing shares with every page (the tokens
including `--ink*`, the palette, the contrast floor, the faces and the one
mono, the logo); this file covers what the landing page shows and says, and
the grammar it adds.

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
moves, the same anime.js motion: stagger, travel, settle, rest, loop.
Calm means no move is shorter than 120ms or longer than 1.1s; travel
eases in and out (`inOutCubic`), a reveal eases out (`outQuad`,
`outCubic`) and an exit eases in (`inQuad`); only a chip or a mark
arriving overshoots (`outBack`); and nothing moves during the rest before
the loop starts again.
Frames of it for reference: record them from the live page before starting.

## Less is more

- Show only what carries the point. A list of fifteen files where two
  matter is noise: the viewer can't find the important ones fast. Show the
  few things that matter (a small group reads instantly, like `.ssh`
  and a cat photo), and cut the rest. What must stay for context but does
  not matter is muted: its text in `--ink-faint`, the faintest grey that
  still holds 4.5:1, its icon in the same grey, never a colour.
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
- No `passwords.csv`: nobody has one, it reads as silly. Private items are
  things people really have (`.ssh`, a cat photo, tax documents).
- Order of the story: the agent first does real, good work (file rows with
  green `+` and red `-` line counts, the way a diff stat looks), running in
  `--dangerously-skip-permissions` mode; snapshots are taken along the way
  (at least two); only then does it go out to the internet and the
  malicious skill comes in. The skill is not on screen from the start. The
  wreck follows, and the machine comes back from the most recent snapshot,
  good work included.
- Show the agent's permission mode the way Claude Code itself does: the
  pink `⏵⏵ bypass permissions on` line at the bottom left of the machine,
  not a `--dangerously-skip-permissions` flag chip.
- No connector lines from the agent to the files it edits; the edits show
  on the file rows themselves (counts appear, rows change).
- Anything that moves exists once: the malicious skill travels from the
  internet (globe) into the machine; no second, faded copy left behind.
- Snapshots are small. The newest shows its time; older ones stack behind
  it as just an icon and a timestamp, contents hidden.
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

- No hand-holding (owner, 2026-09-27): a section head is its title, with
  a sentence only where the picture cannot carry a fact (pricing's rule).
  A feature card gets one line of facts the picture does not show, no
  explanation of the picture. A step is its title and its command. The
  hero's lead is three short sentences: the machine, the wreck, the
  snapshot.

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
- Snapshots are about the machine, not the source. Git already brings back
  deleted source files; a snapshot is the whole disk (the root overlay's
  writable layer and /home, DESIGN.md §6): databases and Docker volumes,
  installed tools and PATH, logins made on the machine, uncommitted work.
  Show damage git can't undo, and the machine back in minutes (a restore
  took about two minutes in real runs; never claim "a minute"). The hero's
  wreck and the snapshot card tell this same story.
- Order of "On every machine": snapshots, localhost, the agent's browser,
  your editor.
- "Break it and roll it back": same aesthetic as the working-state section,
  animated with anime.js. At least two snapshots: one is taken, some edits
  happen, a second is taken and stacks on top, shown only as an icon and a
  timestamp with its contents hidden.
- "Five agents and a full toolchain on first boot" is super clean; leave it.
  2026-09-27: the agents' row shows the marks alone, no name or command
  under them.

## Shape language (owner's direction, 2026-09-27)

The owner brought in a slide template's look (flat geometric shapes in
saturated colours, heavy headings over a thick bar) and asked for it on top
of the house style, not in place of it. Two attempts were sent back: one let
the shapes take over the page, the next added them at the top and bottom
and underlined every heading. The page is one system:

- **Every section is built the same way**: a header (the bold serif
  heading, with a sentence only where the picture cannot carry a fact; see
  "Copy") and then its picture on a stage, the sunken hairline panel
  the grid cards already use. The hero picture and "Your working state"
  sit on a stage too, which keeps the headline apart from the picture.
- **A shape encodes something, or it isn't there.** Owner, 2026-09-27:
  shapes placed "just to have shapes" read as gimmicks, however well they
  are coloured or anchored. The model is Isotype: a shape stands for a
  thing, and its fill or count carries a quantity. Two shapes are
  measures:
  - *Progress* (`Gauge.svelte`): each of the three steps is a circle
    filled a third, two thirds, then whole; grey, and the blue accent for
    the last, "done".
  - *Capacity* (`Units.svelte`): each pricing card counts the memory
    that may run at once in small squares, one per GB (8, 16, 32, in rows
    of eight), Isotype's own form for a quantity, so the plans compare at
    a glance.
  The rest each name one feature and appear where that feature is, so
  the footer's row is the page's own symbols and none "spawns from
  heaven" (owner, 2026-09-27). The mapping, in page order:
  - *pinwheel* (two quarters of a circle: the state before and after) is
    a snapshot. It is the snapshot mark in the hero's snapshots panel and
    on each miniature, in the "Let it break" card, and before that card's
    title.
  - *sphere* is the internet, in the hero, with a globe's meridians drawn
    over it in paper.
  - *pill* is the sync, before "Your working state, in one command".
  - *halves* (the same thing above and below) is the localhost forward,
    before "Your dev server on your localhost".
  - *ring* (a lens: one disc, a paper ring, an ink pupil; the quartered
    ring was redrawn 2026-09-27, its four tones did not work small) is
    watching the agent's browser, before "Watch the agent use the
    browser".
  - *arch* (a door in) is the editor over SSH, before "Open it in your
    editor".
  - *asterisk* (a wildcard) is the toolchain and anything installable,
    before "Five agents and a full toolchain on first boot".
  - *star* is Gemini's sparkle (`SPARKLE` in `marks.ts`) and stands for
    the agents; it is in the toolchain box as Gemini CLI's mark.
  Sun, moon and leaf name nothing on the page and are not shown. A shape
  on a head or a card title sits inline before the words, one em tall, so
  it matches the title's letters (`.head-mark`, `.cell-mark`; owner,
  2026-09-27). The hero's headline carries none.
  A column of agent logos beside the headline was tried and dropped
  (owner, 2026-09-27): a list of logos is a gimmick, and repose is a
  machine for any work, not only AI.
  The footer's row is these eight, in the order the page used them, one
  to a cell between the rails, no pie (it would read as a gauge).
- **Anchored, never floating.** The gauges and counts sit in their line
  or card, the footer's shapes stand on its rule.
- **The blue bar marks "full permissions" and the prices, nothing else.**
  Section headings are bold serif with no bar.
- **Shapes move one way**: a group lands in place when it comes into
  view. The one other shape motion carries meaning (owner, 2026-09-27):
  the snapshot mark clicks a quarter turn when a snapshot is taken and
  rewinds a full turn when one is restored, in the hero and in the "Let it
  break" card. No labels on any of it. Every motion on the page is listed
  under "Motion" below; under `prefers-reduced-motion` every shape is
  still and whole.
- **The palette the landing had before the shapes, and nothing else, and
  no agent's brand colour.** The shapes, the bar and the price rules use
  the neutrals and the one blue accent (`--sh-*` in `layout.css`; blue-500
  for fills, blue-400 in the dark). No Claude Code orange (owner,
  2026-09-27): the landing's own design is not any AI vendor's. No green,
  amber, pink or purple either; the slide template's hues were tried and
  dropped, and a new colour needs a reason recorded here. This rule is for
  what the landing draws for itself. A picture of a real tool keeps that
  tool's colours, because it shows what you will see: Claude Code's orange
  mascot and pink bypass line (above, "The hero"), a terminal's ANSI
  colours, an editor's theme. Those stay inside the picture's frame
  (DESIGN-LANGUAGE.md, "Palette"; DECISIONS I-392).
- **Mostly grey, a spot of colour, even weight.** Each shape has one
  main tone (`tone`: neutral or accent; Shape.svelte); a group carries a
  spot of blue and the rest grey, as the pictures are mostly grey with a
  blue chip. Fills sit
  at mid values so nothing vanishes or shouts: zinc-400 and zinc-300 on
  paper, zinc-500 and zinc-600 in the dark; the blue fill is blue-500
  (a step lighter than the pictures' blue-600 lines) so it doesn't
  outweigh the greys; `--sh-ink` only for small details (a hole, a
  diamond's top), never a whole shape. In the light scheme those details
  are holes in zinc-700; in the dark they are lit marks in zinc-400, since
  a dark detail on the dark paper ring vanished (DECISIONS I-379). Judge
  the balance on viewport captures at 1x in both themes, one per section,
  as "Process" says; a full-page capture only shows the page's rhythm.
- **Where a shape and a mark are the same form, they are one.** The star
  is Gemini's sparkle (`SPARKLE` in `marks.ts`); Gemini CLI's mark in the
  toolchain box is that sparkle.
- **The logo** (`Logo.svelte`, and `static/favicon.png` from the same
  drawing) is the owner's notebook sketch, traced (DECISIONS I-363): a
  thin cross in the text's ink, its crossing left of centre and low, and
  three flat blocks hugging the crossing: a skinny one in grey above-left,
  a middle square in a fainter grey above-right on the arm, and the big
  square in `--sh-accent` below-right. The two greys are the mark's own,
  not `--sh-grey` and `--sh-light`, which vanish at header size; their
  values are in `DESIGN-LANGUAGE.md`, "Logo" (I-393). The favicon
  (`favicon.svg`, `favicon.png`) is a heavier cut of it, stems 10 instead
  of 4, so it holds at 16px. It replaced the r (a stem and a blue quarter
  disc), which had replaced the quartered ring. The mark is in every
  page's header, not only this one's (I-381); where it appears and its
  minimum size are in `DESIGN-LANGUAGE.md`, "Logo".
- The shapes live in `landing/Shape.svelte` and draw only from the `--sh-*`
  tokens. The app's own pages never use them, with one exception: the
  logo's big square is `--sh-accent`, and the logo is in every header
  (I-363, I-381, I-393).

## Motion

Everything that moves on the landing. Each runs only under
`prefers-reduced-motion: no-preference`; with reduced motion the page is
drawn in its final state.

- **The chrome, once on load**: the rails draw from the top down (1.1s,
  `--land-ease`, `cubic-bezier(0.65, 0, 0.35, 1)`); the ticks fade in
  after them (0.4s ease-out, from 0.9s); the blue bar under "full
  permissions" wipes in from the left (0.8s, `--land-ease`, from 0.5s).
  The prices' bar does not move.
- **Shapes landing**: a group lands in place when it comes into view, one
  shape after another (`.land` in `layout.css`: opacity 0.3s ease-out,
  transform 0.7s `cubic-bezier(0.34, 1.56, 0.64, 1)`, a small overshoot,
  each shape delayed by its `--d`).
- **The snapshot mark** turns a quarter when a snapshot is taken and a
  full turn back when one is restored.
- **The pictures** loop in anime.js, the calm motion of "Your working
  state" as the top of this file defines it: stagger, travel, settle, rest,
  loop, each move 120ms to 1.1s.

Nothing else moves. A new motion is added to this list with its duration
and easing, or it does not ship.

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
- Look at captures at their real size. A full-page capture read after
  being scaled to a fifth hides what a visitor sees at once (a lopsided
  hero got through that way, 2026-09-27). Capture the viewport, or crop
  the region, at 1x and judge that; use a full-page capture only for the
  page's rhythm, never for a section's layout.
- When motion could help, it's fine to offer an animated and a static
  version for the owner to choose (anime.js is allowed).

## The page's grammar (design system, 2026-09-27)

The pictures draw a machine as a panel with hairline edges, and the hero
shows those edges as the wall the attack stops at. The page takes that
drawing as its own grammar, in `apps/web/src/routes/landing.css` (imported
by `+page.svelte` alone; the house tokens stay in `layout.css`):

- **Rails.** The content stands between two hairlines (`.rails`) that run
  from the top bar to the footer, 1120px apart at most. They are hidden
  below 768px. On load they draw themselves from the top down, once, and
  the ticks fade in after them ("Motion").
- **Rules run wall to wall, ticked.** Every section (`.sec`) opens with a
  rule across the full width, and a small cross (`::before`/`::after`)
  marks where it meets each rail, as a drawing marks an intersection. The
  top bar's rule is ticked the same way.
- **A section is a head, then a stage.** The head (`SectionHead.svelte`)
  is the bold serif title, and a sentence only where the picture cannot
  carry a fact ("Copy"), inset from the rails by
  `--land-x`; no number and no running label (a "01 Run" label was tried
  and dropped, owner, 2026-09-27: generic). The stage (`.landing-stage`)
  is sunken and fills the width between the rails, so a picture reads as
  a bay inside the walls. The hero's picture sits on a stage the same way.
- **Cells, not cards.** The features (`.cells`/`.cell`), the steps
  (`.step`) and the sizes (`.tier`) are cut by the same hairlines, and the
  dividers cross the full width. Each feature picture is cropped to one
  `.shot` frame; the title and sentence under it belong to the page, not to
  the picture's component.
- **The hero.** One stack on the left edge: the headline, the lead under
  it, then the button and the install command on one row. Nothing is
  pushed to the right; the picture fills the width. The blue bar under
  "full permissions" and on the prices is unchanged. The picture (owner,
  2026-09-27) is your laptop, your cloud machine, and on the right a
  stack: the snapshots panel (titled, the miniatures shrink into it) over
  the internet as a bare 52px globe, no window, since the internet is not
  a machine of anyone's. No snapshot hangs below the machine and the
  globe does not float at the edge. The connectors cross the gap between
  the machine and that stack. The rogue agent is the red mark alone, no
  halo, background or border around it.
- **Commands are rows.** The install command and each step's command are
  one `.cmd`: a mono row on a sunken ground with a hairline, the way a
  picture shows a row of a terminal.
- **The sign-off.** The footer's shape set stands one to a cell between
  the rails (`.frieze`), the rule under it, then the wordmark and links.
  Eight cells in one row at every width, a phone included.
- **Type.** Display `clamp(2.75rem, 6.6vw, 5.25rem)`; section titles
  `clamp(1.9rem, 3.4vw, 2.5rem)`; cell and step titles 1.125rem serif
  600; lead `clamp(1rem, 1.3vw, 1.125rem)`; labels 11px JetBrains Mono,
  0.12em tracking, uppercase (the price's "a month"). Text takes the
  Foundation's three ink steps (`--ink`, `--ink-muted`, `--ink-faint`,
  `DESIGN-LANGUAGE.md`, "Tokens"), set in `layout.css` so they apply
  before `landing.css` arrives (I-331).

Everything in "Shape language" still holds: shapes encode or are absent,
the palette is the neutrals and the one blue, the pictures are untouched.

