# Recording a terminal capture for the landing page

The landing page's terminal panels (`apps/web/src/lib/components/landing/`,
for example `Localhost.svelte` and `Ready.svelte`) show real terminal output,
never text typed by hand (`docs/LANDING.md`). To take one:

1. Run the thing on a real repose machine, in tmux, at the width the panel
   shows.
2. `tmux capture-pane -p -e -J -t <session>:<window>` and paste the rows
   into the component, escapes and all. Crop only whole columns or rows (a
   long prefix, the Nerd Font glyphs after a prompt's directory) and say so
   in the component's header comment, with the machine and the date.
3. Map the colours with the palette in `ops/dev/hero/convert.py` (`BASE16`
   and `xterm256`), which is what the existing panels use, so every capture
   on the page has the same colours.

The hero is no longer a recording: it is anime.js panels (commit `21ff5be`).
The rest of `ops/dev/hero/` (`rec.py`, `schedule.py`, `typeit.py`,
`rshell.sh`, `hold.mjs`, and `convert.py` as a whole) recorded the old tmux
playback for `illustrations/Session.svelte` and its `session.json`, which
are kept but no longer on the page. Their paths point at the scratch
directory of that first recording.
