# Design language

One system for every page: neutral paper and ink, hairline rules, corners
of 2 to 4px, no shadows or gradients, one accent colour. The source is
`apps/web/src/routes/layout.css`; read the CSS, not the recruiting app it
began as a copy of (DECISIONS I-369 retired that).

The Foundation below is shared by the dashboard, the docs, the legal pages
and the landing. The Dashboard part adds the patterns of the signed-in
pages. The landing adds its own grammar in `LANDING.md` on top of the
Foundation, and nothing in the Foundation is restated there.

# Foundation

## Tokens

Scheme colours are CSS variables on `:root` in `layout.css`, redefined
under `prefers-color-scheme: dark`, and exposed as Tailwind utilities
through `@theme inline` (`text-ink`, `border-control`, `bg-sunken`), so
each utility follows the scheme. Light and dark come from the OS only: no
theme toggle, no `class="dark"`, no `[data-theme]`.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--page` | `#fbfbfa` | `#111110` | The page ground (`bg-page`). `app.html` paints it before the CSS loads (I-331). |
| `--surface` | `#ffffff` | `#161615` | Fields, `.btn-quiet`, `.banner`, toasts. |
| `--sunken` | `#f4f4f2` | `#1b1b1a` | Code blocks, the landing's stages, kbd. |
| `--rule` | `#e6e6e3` | `#2a2a28` | Hairlines between sections, rows and cards. |
| `--rule-strong` | `#cfcfcb` | `#3b3b38` | Badges, table heads, banners. Never the only edge of a control. |
| `--control-edge` | `#888883` | `#6a6a66` | The edge of anything you type into or press: fields, `.btn-quiet`, the hollow state dot, the meter track (`border-control`). |
| `--ink` | zinc-900 | zinc-100 | Primary text (`text-ink`). |
| `--ink-muted` | zinc-600 | zinc-400 | Secondary text: metadata, ledes, table heads, the inactive nav link. |
| `--ink-faint` | `#6b6b66` | `#8f8f8a` | Tertiary text: placeholders, code comments, timestamps. |
| `--focus` | blue-600 | blue-400 | The focus ring. |

Pages use `text-ink*`, never a hand-paired `text-zinc-500
dark:text-zinc-400`. That pairing is how the dashboard and the landing
ended up with two different "muted" greys (I-370).

## Palette

Tailwind's zinc, blue, emerald, amber and red are re-toned once in
`@theme`, so a stock utility cannot bring in Tailwind's defaults. Each
colour has one meaning:

- **Zinc** is a neutral grey: text, rules, the primary button.
- **Blue is the accent**, for links (`.link`, the docs' links), the focus
  ring and text selection. Nothing else is blue in the dashboard, the
  docs or the legal pages; a busy state, a current tab and code syntax
  are not (I-373, I-374, I-388). The landing's shapes and bar use it as
  `LANDING.md` says.
- **Emerald** is running and success, **amber** attention (a warning
  banner, a meter over its limit), **red** failure and destruction.

No purple, pink, orange or other hue. A new colour needs a DECISIONS entry.

## Contrast floor

- Text: 4.5:1 or better on `--page`, `--surface` and `--sunken` in both
  schemes. Every `--ink*` token holds it; so does zinc-500 (`#70706b`,
  4.52:1 on light `--sunken`), the lightest grey allowed for small text.
  zinc-400 and zinc-300 are never text.
- Control edges, focus rings, state dots and meter tracks: 3:1 or better
  against what they sit on (WCAG 1.4.11).
- State never rides on colour alone: a dot has its word, a meter says
  "over", a current tab has its underline.
- `tests-a11y/a11y.spec.ts` fails on any Lighthouse binary audit that
  scores 0, on every page in both schemes at 1440 and 390 (I-389).

## Corners

The whole radius scale runs from 2px to 4px (`--radius-xs` and
`--radius-sm` 2px, `--radius-md` 3px, `--radius-lg` and up 4px).
`rounded-sm` on fields, buttons, cards, banners and toasts; `rounded-xs`
on badges, dots and keys. No shadows. No gradients, except the
one-colour `linear-gradient(c, c)` that draws a rule or a bar, and the
landing's sphere.

## Type

Two webfonts, self-hosted from `static/fonts` with their OFL texts, so no
page asks a third party for a font (I-371):

- **Noto Serif** (400, 600, 700) for every heading, `h1` to `h6`, and
  `.font-display`. The base style applies it, so a heading needs no
  `font-display` class.
- **JetBrains Mono** (400, 600, 400 italic) is the one monospace: code,
  commands, ids, sizes, badges and the landing's pictures and labels.
  `--font-mono` leads with it; a component never names its own mono stack.
- **Body text is the system sans** (`--font-sans`, Tailwind's default
  stack written out).

Serif titles over a plain sans page are the identity. Each webfont has a
metric-matched local fallback, so nothing reflows when it arrives.

| Step | Size | Use |
|---|---|---|
| `text-2xs` | 11px | Badges, the landing's uppercase labels. The floor: no text is smaller. |
| `text-xs` | 12px | Table heads, notes under a meter, kbd. |
| `text-compact` | 13px | Mono readings beside sans text, code blocks, the dashboard nav on a phone. |
| `text-sm` | 14px | Dashboard body, fields, buttons, banners. |
| `text-base` | 16px | Docs and legal prose. |
| `text-xl` | 20px | Every dashboard h2, semibold. |
| `text-2xl` | 24px | Docs and legal h2. |
| `text-3xl` | 30px | The page title (`PageShell`'s h1), semibold. |

The landing sets its own display sizes (`LANDING.md`, "Type"). Mono under
13px is a badge or a label of a word or two, never running text.
Pages write no `text-[13px]`; a size that is missing becomes a step here.
Figures that change or line up in columns (sizes, counts, times, prices)
use `tabular-nums`. Page markup uses no `font-bold`; emphasis in body text
is `font-medium` or `font-semibold`.

## Spacing

Tailwind's 4px steps, used the same way everywhere: a 20px side gutter
(`px-5`) at every width; 40px between a page's top and its title
(`pt-10`), 32px from the title rule to the content (`mt-8`), 40px between
sections with 24px under each section's rule (`.form-section`); 20px
inside a card (`p-5`); 12px for a row (`.row`); 96px under the last
section (`pb-24`).

## Motion

Motion is for state, and only on colour, opacity and transform.

| Where | What | Duration, easing | Reduced motion |
|---|---|---|---|
| Buttons, links, nav | Colour on hover | 150ms, Tailwind's default ease | Stays (colour only) |
| `.dot--busy` | Opacity pulse | 2s, `cubic-bezier(0.4, 0, 0.6, 1)`, looping | Still (`motion-safe`) |
| Docs drawer | Slides in from the left | 200ms ease-out | None (`motion-reduce:transition-none`) |
| Docs heading anchor | Fades in on hover | 150ms | Stays (opacity only) |
| Landing | Rails, ticks, the hero's bar, shapes landing, the snapshot mark, the pictures | `LANDING.md`, "Motion" | Every shape still and whole |

Anything that travels, turns or loops runs only under
`prefers-reduced-motion: no-preference`. A colour or opacity change of
200ms or less may stay for everyone.

## Icons

Inline SVG, no icon library: 16px on a 16 or 20 grid, 1.5 stroke, square
caps, no fill, in `currentColor` or zinc-500. Drawn ones today are the
select chevron, the search glass and the docs' fold chevron. A background
image icon disappears in forced colours; the forced-colours block drops
it or swaps in the native control.

## Logo

`Logo.svelte` is the I-363 mark (a thin cross in ink, three blocks in
grey, light grey and the accent) followed by "repose" in Noto Serif 600.
The mark is in every header (I-381):

- 28px tall with the word from `sm` up, on every page.
- Below `sm`: the 24px `sm` cut with the word on the docs and legal pages,
  and the 28px mark alone on the dashboard, whose five links leave no room
  for both. The link around it carries the name.
- Never under 24px. At 16px (the tab) use the favicon, a heavier cut of
  the same drawing (`static/favicon.svg`, `favicon.png`).
- In the dark, the mark's grey blocks are zinc-400 and zinc-500 so they
  hold 3:1 at header size; the landing's large shapes keep `--sh-grey`
  and `--sh-light`.

## Page frame and header

`HeaderFrame.svelte` is the header of the dashboard, the docs and the
legal pages (I-380): 56px tall over a `--rule` hairline, its content on
`mx-auto max-w-5xl px-5`, so the logo sits at x=228 at 1440 and x=20 at
390 on every page. The docs keep it sticky; the others scroll it away.
The right side holds plain text links in `--ink-muted`; the current page
is ink with a 1px underline, no bold shift and no accent colour. No
hamburger on the dashboard; the docs' menu button sits at the right end
below `lg`. The landing's 60px top bar on a 1120px measure is the one
exception, until the landing-critique branch lands.

Every page's content column is `max-w-5xl` under that header. A narrower
column (`max-w-2xl` for forms, 33rem for prose) sits flush left inside it,
starting under the logo, so nothing shifts sideways between pages.

# Dashboard

The signed-in pages: projects, a project and its config and secrets,
billing, settings and account.

## Frame

`PageShell.svelte` wraps every page in a `<main>`: `max-w-5xl px-5 pt-10
pb-24`, with `width="form"` narrowing the content to `max-w-2xl`, flush
left. It draws the breadcrumb (`text-sm text-ink-muted`, `·` separators),
the h1 (`text-3xl font-semibold`), an optional lede and a right-aligned
action, then a `--rule` hairline. A page's h2s are `text-xl
font-semibold`, card titles included; no dashboard page uses h3 (I-375).

## Hairlines, not boxes

Borders use `--rule` for sections, rows and cards, `--rule-strong` for
badges, table heads and banners, and `--control-edge` for controls.
Sections separate with a top rule and spacing (`.form-section`, whose
first one on a page draws no rule under the title's; `.row`). `.card` is a
bordered box with no fill or shadow. `.btn-quiet`, `.banner` and fields
keep a `--surface` fill, one step off `--page`, so a control reads as
something you can use.

## Buttons

- `.btn`: the inverted zinc primary, one per view.
- `.btn-quiet`: `--control-edge` border on `--surface`, for a secondary
  action that still needs a button's weight.
- `.btn-danger`: bordered red, for an action that cannot be undone.
- `.btn-ghost`: text only, muted until hover, for a secondary action.
- `.btn-ghost-danger`: text only, red, for a destructive action that can
  be reversed or that opens a confirmation.

Visual weight tracks consequence. Sizes (I-376): the default suits a
form; `.btn--sm` (`px-3 py-1.5`) is for rows, toolbars and header bars;
`.btn--lg` (`px-5 py-2.5`) for a page's single call to action. No
`!py-*` or `!px-*` overrides. A ghost button at the end of a row takes
`-mr-2` (or `-mx-2`) so its word lines up with the content edge.

## Fields

- `.field`: `rounded-sm`, `--control-edge` border on `--surface`,
  placeholder in `--ink-faint`. Focus is the house `:focus-visible` ring,
  2px `--focus`, offset 2px (I-372); a field does not restyle its border
  on focus.
- `.field--set` darkens the border for a field holding a value the user
  should notice. It is never the focus indicator.
- Every field has a visible `<label for>`, or an `aria-label` when the
  row's heading already names it (a search box). A placeholder is an
  example, never the label.
- An error is a `.field-error` paragraph under the field, with an id; the
  field carries `aria-invalid` and `aria-describedby` pointing at it.
- Selects are `select.field` with the drawn chevron.
- A boolean is a native checkbox (`accent-zinc-900`, dark zinc-100) and a
  text label on one line (`.check-row`). A pick-several list is a
  searchable list of `.check-list-row`s with the description in muted
  text.

## Badges, dots, tables, meters

- `.badge`: 11px mono on a `--rule-strong` edge, neutral. `.badge--new`
  emerald and `.badge--error` red. Nothing else; a tag like "temporary" is
  a plain `.badge`.
- `StateDot.svelte`: an 8px square before the state's word, which is
  always printed. Filled for running (emerald), error (red) and busy
  (ink, pulsing under `motion-safe`); hollow, edged in `--control-edge`,
  for stopped and destroyed (I-373).
- `.table`: hairlines, no fills, heads in `text-xs text-ink-muted`. A
  table that scrolls sideways on a phone sits in a `role=region` with an
  `aria-label` and `tabindex=0`, so a keyboard can scroll it.
- `Meter.svelte`: one series as a thin bar on a `--control-edge` track,
  ink fill, amber past the limit with the word "over" in the reading and
  in `aria-valuetext`. Charts beyond a meter need a DECISIONS entry;
  there is no chart component.

## States

Every view that loads or acts has each of these:

- **Loading**: "Loading…" in `text-ink-muted`, `role=status`. The h1
  names the page ("Project"), never "Loading…".
- **Failed first load**: `LoadState.svelte` shows the error in a
  `.banner--error` (`role=alert`) with a Retry (`.btn-quiet .btn--sm`,
  "Retrying…" while it runs) that re-runs the same load. Only the first
  load goes there; a refresh that fails later keeps the content on screen
  and raises a toast (I-385).
- **Empty**: an h2 that says so ("No projects yet") and one sentence on
  how to get something there, with the command when the CLI is the way.
- **Disabled**: `opacity-50` and `cursor-not-allowed`, and a sentence
  that says why when the reason is not obvious ("Stop todo-app first",
  "320 GB is the largest size."). When no choice is valid, the control is
  replaced by that sentence.
- **Pending**: while an action runs, its button is disabled and its label
  becomes the verb with an ellipsis ("Destroying…", "Re-applying…").
  One action at a time per page section.
- **Danger zone**: last on the page, an h2 in red at `text-xl`
  ("Destroy", "Delete account"), a sentence on what is lost, and
  `ConfirmType`.

## Confirmation

One pattern per consequence, and never the browser's `confirm()` (I-386):

- **Cannot be undone** (destroy a project, delete the account, restore a
  snapshot over the disk): `ConfirmType`. Type the slug or handle; the
  `.btn-danger` stays disabled until it matches exactly; Enter confirms.
  The panel says what is lost.
- **A single deletion whose cost is recoverable** (a secret, cancelling a
  plan): an inline two-step in the row. The first button turns into a
  sentence naming the effect, a `.btn-danger .btn--sm` that does it and a
  `.btn-ghost` "Keep it".
- **Leaving unsaved edits**: the SvelteKit navigation is cancelled and a
  `.banner--warn` asks in place, with Stay (focused) and Leave.

No modals: the inline panel keeps what is being confirmed on screen.

## Tabs

A switch that swaps a panel in place without changing the URL is ARIA
tabs: `tablist`, `tab` with `aria-selected` and a roving tabindex,
`tabpanel` with `aria-labelledby`; arrows, Home and End move between
tabs. The current tab looks like the header's current page: ink with a
1px underline, no weight change, no accent (I-388). A switch that changes
the URL is links with `aria-current`.

## Toasts

`svelte-sonner`, `theme="system"`, bottom-right, without `richColors`
(I-374). A toast is a banner of its kind: `--surface` and `--ink` for
neutral, the `.banner--ok`, `--error` and `--warn` colours for typed
ones, 2px corner, a hairline, no shadow. Toasts report the result of
something the user did, and a failed refresh. A failure that leaves the
page with nothing to show is a banner, not a toast.

## Forced colours

An unlayered `@media (forced-colors: active)` block in `layout.css` puts
back whatever carries a state in a fill or a coloured border (I-377):
state dots, the current nav item, select arrows, button, badge and key
edges, disabled buttons in `GrayText`, and the meter fill
(`Meter.svelte`). A new component that shows state that way adds its rule
there and is checked with Chromium's `forcedColors: 'active'`.

## Replace

For boolean and pick-one settings (notification channels on or off, hold
base updates, default agent, size class) use plain controls in the same
palette, not chips or segmented groups:

- A boolean is a native checkbox and a label on one line (`.check-row`).
  One per line, aligned left.
- A pick-one with a handful of options is a `select.field`. A pick-one
  that needs a sentence per option is a vertical list of native radios
  with the help text under each; no card borders around options. No page
  needs one today, and the old `.radio-row` classes were deleted (I-378);
  build it again in `layout.css`, with forced-colour states, when one
  does.
- Pick-several (the config menu's package list) is a searchable list with
  a native checkbox per row and the description in muted text, not a wall
  of pills.

Chip-wrapped checkboxes, segmented pick-ones and card-sized radio options
stay rejected. So does a toggle switch until a page needs one (the unused
`.switch` was deleted with the rest).

# Docs and legal pages

They use the Foundation and the shared header. Prose is `.doc` in
`layout.css`: the site's palette over `@tailwindcss/typography`, inline
code as a quiet chip at body weight with no backticks, links in the
accent, h2 sections separated by a rule. Running text holds to 33rem;
the docs column is 68ch so code blocks and tables get the full 70 columns
I-345 writes to (I-382). The docs list the current page's sections under
its link in the sidebar from `lg` up and in an "On this page" fold below
(I-383). Legal pages show their effective date under the title.

# The landing's exception

The landing adds a shape set in the `--sh-*` tokens (zinc greys, ink and
the blue accent), a blue bar under its headline and prices, a sphere
drawn with SVG noise and a gradient, its own display sizes, and its own
page grammar (rails, ticked rules, stages, cells) in
`apps/web/src/routes/landing.css`. Those exist only on `/` and are
described in `LANDING.md`. Every other page follows this document with no
exception.

# Where it goes

`apps/web/src/routes/layout.css` for tokens and classes; `app.html` for
the font preloads and the pre-CSS colours, which must match `--page` and
`--ink`. A new class goes in `layout.css` with a comment that says why,
in full sentences, like the rest of the file. `CHECKLIST.md`, "Design",
has the greps that catch drift from this document.
