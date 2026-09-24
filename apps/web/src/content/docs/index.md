---
title: Overview
description: What repose is and how the pieces fit together.
section: Start here
order: 1
---

repose gives each of your projects its own Linux machine in the cloud. You start a coding agent there from your laptop's terminal, close the laptop, and the agent keeps working. It sends you a notification once it finishes or needs an answer, and you pick the session back up from any computer.

The agent runs with full permissions on that machine. It can install packages, run Docker, delete files and push to your branch. It has no network path to your laptop or to your other projects, and your SSH private keys are never copied to it. [Security](/docs/security) lists what does reach the machine, including the one thing that reaches back while you're attached.

## The pieces

You work with three things.

**The `repose` command** runs on your laptop (macOS or Linux). It creates the machine, copies your uncommitted work and your tool logins onto it, and drops you into a tmux session there. Almost everything you do goes through it.

**The machine** is a NixOS microVM, one per project, with its own kernel, disk and Docker daemon. You log in as the user `dev`. Your checkout sits at `/home/dev/<project>`. Five coding agents, Node, Python, Go, Rust, a C toolchain, headless Chromium and the usual command-line tools are already installed. The machine stays on until you stop it.

**The dashboard** at [repose.herakraft.co](https://repose.herakraft.co) shows every project's state and cost, lets you add packages from a menu, manage secrets and snapshots, set up notifications and pay.

## A typical session

```
$ cd ~/code/todo-app
$ repose run "add rate limiting to the login route, run the tests, commit when green"
```

The first time, this creates a machine for the checkout, builds its environment and boots it. Then it copies over your uncommitted edits and any commits you haven't pushed, starts Claude Code in a tmux window, types your prompt into it and attaches you to that window.

Press `Ctrl-b` then `d` to detach. The agent keeps going. Close the laptop if you like.

Later, from the same laptop or another one:

```
$ repose attach todo-app
```

You're back in the same tmux session, with the agent's full history on screen.

Once the work is ready, the agent commits and pushes from the machine the way you would. You pull on your laptop. Nothing syncs back on its own.

## Where to go next

- [Quickstart](/docs/quickstart) takes you from nothing to a running agent in about five minutes.
- [Run and attach](/docs/run-and-attach) covers the tmux session and how agents are started.
- [What gets synced](/docs/sync) lists which files travel to the machine and which never do.
- [Ports and localhost](/docs/ports) explains how a dev server on the machine shows up on your laptop.
- [Pricing and billing](/docs/billing) has the rates and what a stopped machine costs.
