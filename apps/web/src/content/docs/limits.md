---
title: Limits and acceptable use
description: How many projects you can have, what the network allows, and what gets a machine stopped.
section: Account
order: 31
---

## Projects

A new account can have 3 projects, at most 1 of them `xl`. After your first paid invoice, the limit is 10 projects of any size. Destroyed projects don't count. Each copy [`repose fork`](/docs/lifecycle#fork-a-project) makes is a project.

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
- host or run anything illegal.

A machine running a known miner is stopped, with a snapshot. You get a notification, and `repose status` and the dashboard say why. After three stops in a day the project can't be started until we've looked at it. Other abuse found by monitoring or reported to us gets the machine stopped and the account reviewed. Monitoring looks at process names and resource use, never at your files or terminal; see [what repose stores](/docs/secrets#what-repose-stores).
