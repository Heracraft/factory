<!-- The machine guide every agent on a repose machine reads (DECISIONS I-243).
agent-guide.nix renders it: HTML comments are dropped, and a line marked
"needs: CMD" is dropped while the guest has no CMD. Each line names the /docs
section it summarises. internal/cli/agent_guide_test.go fails when a section
of machine.md, agents.md or limits.md is referenced by no line here, when a
reference has no such page or heading, and when a command named here is not
on the machine. Keep it short and factual; the user reads it too. -->
# This machine

This is a repose machine: a NixOS virtual machine for one project, where agents keep working after the user's laptop closes. <!-- /docs/machine -->
The user works from their laptop. You cannot reach the laptop or its files from here; what they should see has to be on this machine, in git, or sent with the commands under "Reaching the user". <!-- /docs/secrets#what-an-agent-on-the-machine-can-reach -->
You are `dev`, with passwordless `sudo`. The checkout is under `/home/dev`, and everything in `/home/dev` survives a stop. <!-- /docs/machine -->

## Servers and ports

- While the user is attached, every port a program here listens on (1024 and up, on `localhost` or `0.0.0.0`) appears on their laptop at the same port within a second or so. Tell them "open http://localhost:PORT". <!-- /docs/machine#ports -->
- There are no public URLs. Don't look for one or start a tunnel unless the user asks. <!-- /docs/machine#ports -->
- Not forwarded: ports below 1024, 5353, 5355, 5900, 6080 and 6081, and servers that listen only on another address. A Docker port published with `-p` is forwarded. <!-- /docs/machine#ports -->
- If the user isn't attached, they can forward one port with `repose open PORT` on their laptop. <!-- /docs/machine#ports -->

## Installing tools

- Already installed: Node.js 24 with npm and pnpm, Python 3.12 with uv, Go, rustup, gcc, make, cmake, Docker, Chromium, git, gh, jq, ripgrep, sqlite3, psql and the usual command-line tools. <!-- /docs/machine#whats-installed -->
- Programs built for other Linux systems (prebuilt binaries, Prisma engines, Python wheels, `curl | sh` installers) run as they would on Ubuntu. <!-- /docs/machine#whats-installed -->
- Install a missing tool now with `nix profile add nixpkgs#NAME`. Typing a missing command prints the package that has it. `npm i -g`, `go install` and `uv tool install` work too. All of these stay on this machine's disk. <!-- /docs/machine#installing-more -->
- Installs made here are not part of the project's configuration. To keep a package on every rebuild, tell the user to run `repose config add NAME` on their laptop. <!-- /docs/config#add-a-package -->
- Tools the user has on their laptop are installed in the background after each `repose run`. If one is missing right after a start, check `~/.repose/tools-install.log` before installing it yourself. <!-- /docs/machine#your-laptops-tools-come-along -->
- npm, pnpm, yarn v1 and Docker Hub downloads go through a cache on the server. Leave the two lines repose added to `~/.npmrc` in place. <!-- /docs/machine#network -->
- For a repository with a `flake.nix`, put `use flake` in `.envrc` and run `direnv allow`. <!-- /docs/machine#projects-with-a-flake-nix -->

## Docker and databases

- `docker` and `docker compose` work without `sudo`. <!-- /docs/machine#whats-installed -->
- No database server is installed. Run one in Docker (`docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=dev postgres:17`), or tell the user to run `repose config add postgresql` on their laptop (also `redis`, `mysql` and others), which starts it on `localhost`. <!-- /docs/config#the-menu -->

## Secrets

- The user's secrets are environment variables in your shell and files in `/run/repose/secrets/`. <!-- /docs/secrets#store-an-api-key -->
- Never print, log or commit a secret's value, and never write one into the repository. Refer to it by name, as `$NAME`. <!-- /docs/secrets#store-an-api-key -->
- If you need a secret that isn't set, ask the user to run `repose secrets set NAME` on their laptop. New values reach new shells; restart a running server to pick one up. <!-- /docs/secrets#store-an-api-key -->

## Browser

- Drive a browser with the `playwright` or `chrome-devtools` MCP tools when you have them (Claude Code does), or with Playwright from code. Its browsers are installed; skip `npx playwright install`. <!-- /docs/machine#browser -->
- The `playwright` and `chrome-devtools` tools share one browser, which the user sees live when they run `repose open --desktop` on their laptop. For a step only a person can do (a captcha, a passkey, a login), ask them to open the desktop and do it in that browser; its logins are kept. <!-- /docs/machine#browser -->

## Memory and disk

- When memory runs out, test runs and dev servers are killed before agents and tmux. `sudo dmesg | grep -i killed` shows what went. <!-- /docs/machine#memory-and-disk -->
- `df -h /home/dev` shows free disk. The user can grow it with `repose resize 80G` (any size) on their laptop. <!-- /docs/machine#memory-and-disk -->
- If processes keep getting killed for memory, tell the user: `repose resize --size large` (or `--size xl`) on their laptop gives the machine more memory. It restarts the machine, which ends every process here, you included. <!-- /docs/machine#changing-the-size -->

## Reaching the user

- The user is notified when you finish or wait for input. You don't have to do anything for that. <!-- /docs/notifications#what-youll-get -->
- To tell the user something while they're away, run `repose-notify "MESSAGE"`. It reaches their phone or email. <!-- /docs/notifications#agents-can-message-you-and-ask-questions --> <!-- needs: repose-notify -->
- When you're blocked on a decision only the user can make, run `repose-ask --options yes,no "QUESTION"` (or without `--options` for a free answer). It waits up to 30 minutes (`--timeout`) and prints their answer; exit 3 means no answer came, 4 that they have no notifications set up. Don't ask what you can decide yourself. <!-- /docs/agents#let-it-ask-you --> <!-- needs: repose-ask -->

## Git

- Push over HTTPS. When the user's `gh` login was copied, `git push` to github.com works, and `git@github.com:` remotes are rewritten to HTTPS. There is no SSH key on this machine. <!-- /docs/secrets#logins-copied-from-your-laptop -->
- Commits made here are unsigned; the signing key stays on the laptop. <!-- /docs/secrets#git-and-claude-code-settings -->
- If you were started in a folder next to the checkout (`~/PROJECT-claude-2` and the like), it is a git worktree on its own branch (repose/claude-2 and the like), so other agents' files are not yours. Commit your work on that branch; don't copy it into the checkout. <!-- /docs/run-and-attach#several-agents-separate-trees -->

## Agents

- Claude Code (`claude`), Codex (`codex`), opencode (`opencode`), Gemini CLI (`gemini`) and pi (`pi`) are installed. <!-- /docs/agents -->
- If an agent says it isn't logged in, ask the user to log it in on this machine; logins are never copied here for Claude Code. <!-- /docs/agents#log-in -->
- `~/.claude/CLAUDE.md`, settings and skills are copied from the user's laptop at each `repose run`, so lasting changes to them belong on the laptop. <!-- /docs/agents#your-claude-code-setup-comes-along -->
- MCP servers that need the laptop (Apple Notes, Xcode, Claude in Chrome) don't work here. HTTP servers and `npx` servers do; put their tokens in secrets and refer to them as `${NAME}`. <!-- /docs/agents#mcp-servers -->
- Images the user pastes with `repose paste` are saved in `/tmp/repose-paste/`, and the file's path is pasted into your prompt. Claude Code attaches it; other agents can open the file. <!-- /docs/run-and-attach#paste-an-image -->

## Limits

- Nothing on the internet can connect to this machine. Outbound traffic is allowed, up to 200 Mbit/s. <!-- /docs/limits#network --> <!-- /docs/machine#network -->
- Outbound port 25 is blocked. Send mail through a provider's API or its submission port (587 or 465). <!-- /docs/limits#network -->
- New outbound connections are limited to 200 a second, in bursts of up to 2000. <!-- /docs/limits#network -->
- Don't mine cryptocurrency, send bulk mail, or scan or flood other systems. A known miner gets the machine stopped. <!-- /docs/limits#what-isnt-allowed -->
