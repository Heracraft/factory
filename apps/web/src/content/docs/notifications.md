---
title: Notifications
description: Get told by email or on your phone when an agent finishes or needs you.
section: Using repose
order: 16
---

## Email

Email is on from the start and goes to the address on your account. Each message has a link that turns email off. To switch it yourself:

```
repose notify set --email off
repose notify set --email on
```

## Your phone, with ntfy

[ntfy](https://ntfy.sh) is a free push notification service: you subscribe to a topic in its app, and anything posted to that topic reaches your phone.

1. Install ntfy from the [App Store](https://apps.apple.com/app/ntfy/id1625396347) or [Google Play](https://play.google.com/store/apps/details?id=io.heckel.ntfy).
2. Pick a topic name nobody could guess. Topics on ntfy.sh are readable by anyone who knows the name, so make it long and random: `openssl rand -hex 10` gives a good suffix.
3. Subscribe to it in the app.
4. Tell repose, and send a test:

```
repose notify set --ntfy https://ntfy.sh/repose-4f9c2a7e1b3d5c8a0f6e
repose notify test
```

```
email: ok
ntfy: ok
```

A self-hosted ntfy server works the same way. For one that needs a login, put it in the URL (`https://user:password@ntfy.example.com/topic`). Turn ntfy off with `repose notify set --ntfy none`.

Settings apply to every project. The dashboard's **Settings** page has the same controls.

## What you'll get

| Title                           | When                                                     |
| ------------------------------- | -------------------------------------------------------- |
| `todo-app: claude finished`     | The agent finished and is waiting.                       |
| `todo-app: claude needs input`  | The agent is asking you something, usually a permission. |
| `todo-app: claude hit an error` | The agent reported an error.                             |
| `todo-app: snapshot failed`     | A snapshot couldn't be taken.                            |
| `todo-app: base update failed`  | A platform update didn't build with your configuration.  |

The body is what the agent said at that moment, up to 1 KB. It's never your prompt or your terminal. It does pass through ntfy.sh or your email provider, so use a self-hosted ntfy server if that matters.

Claude Code, Codex and opencode report through hooks, within about 10 seconds. Gemini CLI and pi have no hooks, so the machine sends `finished` when their processes go quiet, within about 90 seconds.

## Limits

Repeats of the same event from the same agent within 60 seconds are sent once. A project sends at most 30 notifications an hour; past that, one message says they're paused until the next hour.

## Check what happened

```
repose events todo-app
repose events todo-app --since 72h -f
```

If an event is listed but nothing arrived, run `repose notify test`. An `error` there means the channel's settings are wrong.
