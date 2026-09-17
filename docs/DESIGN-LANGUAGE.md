# Design language for the dashboard

Copied from the recruiting app (`../recruiting/apps/web/src/routes/layout.css`
and its components), which is the house style. Copy the typography, palette,
spacing, buttons and text fields. Do **not** copy its non-text controls: the
chip-wrapped checkboxes and radios, the segmented pick-one, and the card-sized
radio option (`.chip:has(input)`, `.seg`, `.option`, `SegmentedInput.svelte`,
`DeliveryModeChoice.svelte`, `CheckboxField.svelte`). Those are rejected.

## Keep

- **Tailwind v4** with `@theme` in one `layout.css`, `@tailwindcss/typography`
  for prose. No component library.
- **Headings in Noto Serif**, loaded from Google Fonts with weights 400, 600,
  700, exposed as `--font-display` and applied to `h1` globally and to other
  headings with the `font-display` class. Body text stays the Tailwind
  default sans stack. This pairing is the identity: serif titles over a
  plain sans page.
- **Zinc palette, light and dark from the OS only.** `color-scheme: light
  dark`, body `bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100`,
  no theme toggle, no `class="dark"`.
- **One accent: blue-700 (dark: blue-400)** for links, the current nav
  item, focus rings and "this field carries a value". Nothing else is blue.
- **Borders, not boxes.** Sections separate with `border-t border-zinc-200
  dark:border-zinc-800` and spacing (`.form-section`), not with filled cards.
  Filled surfaces are reserved for fields and the primary button.
- **Buttons**: `.btn` inverted zinc primary; `.btn-danger` bordered red for
  irreversible actions; `.btn-ghost` text-only for secondary; `.btn-ghost-
  danger` for reversible-destructive. Visual weight tracks consequence.
- **Text fields**: `.field` (rounded-lg, zinc border, white or zinc-900
  fill, blue focus ring), `.field--set` when carrying a value, `.field-error`
  under it. Selects use `.field` with the inline chevron SVG.
- **Badges**: `.badge` neutral, `--new` emerald, `--warn` amber, `--error`
  red, `--info` outline. Amber on most rows means nothing, so use it rarely.
- **Page frame**: `PageShell.svelte`, `max-w-2xl` for forms, `max-w-3xl` for
  lists, `px-5 pb-20 pt-10`, breadcrumb in `text-sm text-zinc-500` with `·`
  separators, optional right-aligned action.
- **Header**: one bordered row, wordmark in `font-display`, plain text nav
  links in zinc with the current page in the accent colour, no bold shift
  on the active item, stacks to two rows on phones instead of a hamburger.
- **Toasts**: `svelte-sonner` with `theme="system"`, bottom-right.
- **Comments in the CSS say why**, in full sentences, like the source.

## Replace

For boolean and pick-one settings (notification channels on or off, hold
base updates, default agent, size class) use plain controls in the same
palette, not chips or segmented groups:

- A boolean is a native checkbox with `accent-blue-700` and a text label on
  one line, or a small switch component drawn with zinc borders and the
  blue accent when on. One per line, aligned left.
- A pick-one with a handful of options is a `select.field`. A pick-one that
  needs a sentence per option is a vertical list of native radios, each
  with its label and a `text-sm text-zinc-500` explanation underneath. No
  card borders around options.
- Pick-several (the config menu's package list) is a searchable list with a
  native checkbox per row and the row's description in muted text, not a
  wall of pills.

## Where it goes

`apps/web/src/routes/layout.css` and `apps/web/src/app.html` (font link).
Workstream 08 copies the "Keep" classes from the recruiting file, deletes
the "Replace" ones, and writes new ones with the same comment style.
