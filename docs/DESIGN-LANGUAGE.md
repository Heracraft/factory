# Design language for the dashboard

The source is `apps/web/src/routes/layout.css` (the restrained redesign,
630cde8): neutral paper and ink, hairline rules, corners of 2 to 4px, no
shadows or gradients, one accent colour. It began as a copy of the
recruiting app's style and no longer follows it; read the CSS, not that
app. Chip-wrapped checkboxes, segmented pick-ones and card-sized radio
options stay rejected.

## Keep

- **Tailwind v4** with `@theme` in one `layout.css`, `@tailwindcss/typography`
  for prose. No component library.
- **Headings in Noto Serif**, loaded from Google Fonts with weights 400, 600,
  700, exposed as `--font-display` and applied to every heading (`h1` to
  `h6`) and the `font-display` class. Body text is the system sans; code,
  commands and ids the system mono (`--font-mono`). This pairing is the identity: serif titles over a
  plain sans page.
- **Neutral palette, light and dark from the OS only.** `color-scheme:
  light dark`; the page, surface and rule colours are CSS variables on
  `:root` (`--page`, `--surface`, `--sunken`, `--rule`, `--rule-strong`)
  redefined under `prefers-color-scheme: dark`. Tailwind's zinc, blue,
  emerald, amber and red scales are re-toned in `@theme`, so zinc is a
  neutral grey. No theme toggle, no `class="dark"`.
- **One accent, the re-toned blue**, for links (`.link`), focus outlines
  (`:focus-visible`, blue-600 / blue-400) and text selection. Nothing
  else is blue.
- **Corners of 2 to 4px**: `rounded-sm` on fields, buttons, cards and
  banners, `rounded-xs` on badges, dots and keys. No shadows, no
  gradients.
- **Hairlines, not boxes.** Borders use `--rule` (sections, rows) or
  `--rule-strong` (fields, buttons, badges). Sections separate with a top
  rule and spacing (`.form-section`, `.row`); `.card` is a bordered box
  with no fill or shadow.
- **Buttons**: `.btn` inverted zinc primary; `.btn-quiet` bordered with no
  fill, for a secondary action that still needs a button's weight;
  `.btn-danger` bordered red for irreversible actions; `.btn-ghost`
  text-only for secondary; `.btn-ghost-danger` for
  reversible-destructive. Visual weight tracks consequence.
- **Text fields**: `.field` (`rounded-sm`, `--rule-strong` border, focus
  turns the border zinc-900 / zinc-300 with no ring), `.field--set` when
  carrying a value (same dark border), `.field-error` under it. Selects
  use `.field` with the inline chevron SVG.
- **Badges**: `.badge` neutral, `--new` emerald, `--warn` amber, `--error`
  red, `--info` outline. Amber on most rows means nothing, so use it rarely.
- **Page frame**: `PageShell.svelte`, `max-w-2xl` for forms, `max-w-5xl`
  otherwise, `px-5 pt-10 pb-24`, breadcrumb in `text-sm text-zinc-500` with `·`
  separators, optional right-aligned action.
- **Header** (`Header.svelte`): one row over a `--rule` hairline, the logo
  on the left, plain text nav links in zinc-500; the current page is ink
  with a 1px underline, no bold shift and no accent colour. No hamburger.
- **Toasts**: `svelte-sonner` with `theme="system"`, bottom-right.
- **Comments in the CSS say why**, in full sentences, like the source.

## Replace

For boolean and pick-one settings (notification channels on or off, hold
base updates, default agent, size class) use plain controls in the same
palette, not chips or segmented groups:

- A boolean is a native checkbox (`accent-zinc-900`, dark `zinc-100`) and
  a text label on one line (`.check-row`), or a `.switch` drawn with zinc
  borders that fills zinc-900 when on. One per line, aligned left.
- A pick-one with a handful of options is a `select.field`. A pick-one that
  needs a sentence per option is a vertical list of native radios
  (`.radio-row`, `.radio-row-label`, `.radio-row-help`). No card borders
  around options.
- Pick-several (the config menu's package list) is a searchable list with a
  native checkbox per row and the row's description in muted text, not a
  wall of pills.

## Where it goes

`apps/web/src/routes/layout.css` and `apps/web/src/app.html` (font link).
A new class goes in `layout.css` with a comment that says why.
