# Recording the landing page's hero

The hero (`apps/web/src/lib/components/illustrations/Session.svelte`) plays
back a real tmux session, captured frame by frame; nothing in it is drawn by
hand. To record a new one:

1. Clone a real project to scratch (`git clone --shared`), `pnpm install`, and
   set `clearScreen: false` in its `vite.config` (committed in the clone only)
   so Vite does not wipe the pane on start.
2. `tmux new-session -d -s rec -x 150 -y 36 "claude --dangerously-skip-permissions"`
   (the footer then reads "bypass permissions on", which the page advertises),
   `tmux set -g focus-events on`, `tmux set -t rec status off`, then
   `tmux split-window -h -t rec:0 -l 62 ops/dev/hero/rshell.sh`. A repose
   guest ships `mouse on` and `focus-events on` (`nix/guest/base/tmux.nix`);
   match it so Claude Code prints no tmux hint.
3. Put pnpm at the guest's version first on PATH in `rshell.sh` (read it on a
   guest with `node -v; pnpm -v; go version`), so the version check shown is
   what a guest prints.
4. Start `ops/dev/hero/rec.py 300` (captures up to three panes with `-e -J`
   every 200 ms). Type `pnpm dev` into `rec:0.1` with
   `ops/dev/hero/typeit.py rec:0.1 "pnpm dev" enter`, open the app in a
   browser (`hold.mjs`) so Vite logs reloads, split that pane with
   `tmux split-window -v -t rec:0.1 -l 12 ops/dev/hero/rshell.sh`, type
   `node -v && pnpm -v && go version` and `eza` into `rec:0.2`, then type the
   prompt into `rec:0.0`.
5. When Claude prints its `… for 59s · done` footer, run `convert.py` (ANSI to
   styled runs, the scratch path rewritten to `/home/dev/…`, lines re-wrapped
   to the pane widths) and `schedule.py` (real-time typing, the session
   compressed, the detach frame appended). It writes `session.json` next to
   the component.

Paths in the scripts point at the scratch directory used for the first
recording; adjust them.
