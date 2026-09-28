---
title: Limits and acceptable use
description: How many projects you can have, what the network allows, and what gets a machine stopped.
section: Account
order: 31
---

## Projects

A new account can have 3 projects, at most 1 of them `xl`. After your first paid invoice, the limit is 10 projects of any size. Destroyed projects don't count, and neither does one still being destroyed. A project whose destroy failed still counts until `repose rm` succeeds. Each copy [`repose fork`](/docs/lifecycle#fork-a-project) makes is a project.

## When repose is full

Machines never share memory, so there's room for a fixed number of them. When the servers are close to full, a new account's first project waits: `repose run` says `repose is at capacity. You're number 3 on the waitlist; we'll email you@example.com when there's room.` and exits with code 8. Running it again keeps your place. The dashboard shows your place too.

We let people in, in the order they joined, as room frees up or we add a server. You get one email when it's your turn; it's sent even if you've turned notification emails off. Then run `repose run` again. Once you're in, or once you've had a project, you never wait in this queue again.

## Network

- Outbound traffic is limited to 200 Mbit/s per machine. Downloads into the machine aren't limited.
- Outbound connections to port 25 are blocked, so a machine can't send mail directly. Use your email provider's API, or its submission port (587 or 465) with a login.
- Outbound connections to the ports mining pools use (3333, 5555, 7777, 14433 and 14444) are blocked.
- A machine can open 200 new outbound connections a second, in bursts of up to 2000. Installing packages, running test suites and crawling your own app stay well under it.
- Nothing on the internet can connect to the machine. Reach your own servers on it through [port forwarding](/docs/machine#ports).

## What isn't allowed

The [terms](/terms) have the full wording. In short, don't use a machine to:

- mine cryptocurrency;
- send spam or bulk mail;
- attack, scan or flood other systems;
- host a live product for other people (showing someone work in progress is fine);
- host or run anything illegal.

A machine running a known miner is stopped, with a snapshot. You get a notification, and `repose status` and the dashboard say why. After three stops in a day the project can't be started until we've looked at it. Other abuse found by monitoring or reported to us gets the machine stopped and the account reviewed. Monitoring looks at process names and resource use, never at your files or terminal; see [what repose stores](/docs/secrets#what-repose-stores).
