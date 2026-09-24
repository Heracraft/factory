---
title: Notifications
description: Getting told when an agent finishes or needs you, by email or on your phone with ntfy.
section: Using repose
order: 16
---

repose can tell you each time an agent finishes, asks a question or hits an error, by email, by [ntfy](https://ntfy.sh) push notification, or both. Email is on from the start. ntfy is off until you give it a URL.

## Email

Messages go to the address on your account (the dashboard's Account page shows it). A message looks like this:

```
Subject: [repose] todo-app: claude finished

todo-app: claude finished

Added the rate limiter and its tests. 14 tests pass. Committed as 3f9e2a1.

Attach with `repose attach --project todo-app` or open https://repose.herakraft.co/projects.

Stop these emails: https://...
```

The last link turns email notifications off without logging in. Turn them back on (or off) with:

```
repose notify set --email on
repose notify set --email off
```

## ntfy on your phone

ntfy is a free, open-source push notification service. You pick a topic name, subscribe to it in the ntfy app, and anything posted to that topic shows up on your phone.

1. Install ntfy on your phone from the [App Store](https://apps.apple.com/app/ntfy/id1625396347) or [Google Play](https://play.google.com/store/apps/details?id=io.heckel.ntfy), or use the web app at ntfy.sh.
2. Make up a topic name that nobody could guess. Topics on ntfy.sh are public to anyone who knows the name, so use something long and random, such as `repose-` followed by 20 random characters. `openssl rand -hex 10` makes a good suffix.
3. In the app, subscribe to that topic.
4. Tell repose:

```
repose notify set --ntfy https://ntfy.sh/repose-4f9c2a7e1b3d5c8a0f6e
repose notify test
```

`notify test` sends a test message on every channel that's switched on and prints how each went:

```
email: ok
ntfy: ok
```

A self-hosted ntfy server works the same way; use its URL. For a server or topic that needs a login, put the credentials in the URL (`https://user:password@ntfy.example.com/topic`) and they're sent as HTTP basic authentication. The URL is stored with your settings, so treat it as you would a password.

To stop ntfy notifications:

```
repose notify set --ntfy none
```

You can also change all of this on the dashboard's [Settings](https://repose.herakraft.co/settings) page. Settings are per account. Every project uses the same channels.

## What you'll get

| Title                                    | When                                                                                              | ntfy priority |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------- |
| `todo-app: claude finished`              | The agent finished a turn and is waiting.                                                         | default       |
| `todo-app: claude needs input`           | The agent is asking you something, usually a permission.                                          | urgent        |
| `todo-app: claude hit an error`          | The agent reported an error.                                                                      | high          |
| `todo-app: snapshot failed`              | A snapshot couldn't be taken.                                                                     | high          |
| `todo-app: destroy failed`               | A destroy you started couldn't finish. `repose destroy` tries again.                              | high          |
| `todo-app: host moved`                   | The machine's server was lost and the project was restored onto another from its latest snapshot. | default       |
| `todo-app: base updated`                 | The machine was moved to a new platform base.                                                     | default       |
| `todo-app: base update failed`           | A new base wouldn't build with your configuration. Nothing changed on the machine.                | high          |
| Your guests were stopped for non-payment | Machines were stopped three days after a failed payment.                                          | high          |

The message body is what the agent itself said at that moment, cut to 1 KB. It's never your prompt and never the contents of your terminal. Tapping an ntfy notification opens your project list in the dashboard.

Keep in mind that the body passes through ntfy.sh (or your own server) and your email provider. If an agent's last message could contain something sensitive, use a self-hosted ntfy server or turn ntfy off for that period.

## How agents report

Claude Code, Codex and opencode have hooks, and each machine registers repose's in their configuration. Their notifications arrive within about 10 seconds.

Gemini CLI and pi have no hooks. For them, the machine watches whether the agent's processes are using CPU. After a while with no activity, it sends one notification titled `todo-app: gemini finished` whose body reads `gemini went idle`. That usually means it finished or is waiting for you; the machine can't tell which, and the body says so. These arrive within about 90 seconds. Nothing reads the text on the screen to decide this.

## Limits

- The same kind of event from the same agent in the same project within 60 seconds is sent once. A later summary is added to the first one.
- At most 30 notifications per project per hour. Past that you get one message saying notifications are paused until the top of the hour. The events are still recorded, and `repose events` shows them.

## Checking what happened

```
repose events todo-app
```

lists the project's events from the last 24 hours with times, agents and summaries. `--since 72h` goes further back, `-f` keeps watching, and `--json` prints raw records. The project page in the dashboard shows the same events.

If an event is listed but you didn't get a notification, run `repose notify test`. An `error` there means the channel itself is failing (a mistyped URL, a topic that needs a login). If the test works and a real event still didn't arrive, the rate limit above is the usual reason.

## Remote Control and other agent features

Notifications from repose don't replace the agents' own features. If you logged in to Claude Code on the machine with a Claude subscription, Remote Control works, and you can open the session in the Claude app. Claude Code's own integrations work if you set them up on the machine.
