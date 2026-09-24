---
title: Notifications
description: Get told by email or on your phone when an agent finishes, needs you, or asks you something, and answer from there.
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

Settings apply to every project. The dashboard's **Settings** page has the same controls, plus **Send test**; changes there take effect when you press **Save**.

## What you'll get

| Title                            | When                                                                                                                                                                    |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `todo-app: claude finished`      | The agent finished and is waiting.                                                                                                                                      |
| `todo-app: claude needs input`   | The agent is asking you something, usually a permission.                                                                                                                |
| `todo-app: claude hit an error`  | The agent reported an error.                                                                                                                                            |
| `todo-app: snapshot failed`      | A snapshot couldn't be taken.                                                                                                                                           |
| `todo-app: base update failed`   | A platform update didn't build with your configuration.                                                                                                                 |
| `todo-app: base updated`         | A platform update was switched into the machine.                                                                                                                        |
| `todo-app: destroy failed`       | A destroy didn't finish; the body says why.                                                                                                                             |
| `todo-app: host moved`           | The project was restored onto another server from its latest snapshot.                                                                                                  |
| `todo-app: abuse stopped`        | The machine was stopped because a miner was running ([Limits](/docs/limits#what-isnt-allowed)). By email: `Your guest was stopped: a cryptocurrency miner was running`. |
| `todo-app: claude says`          | An agent, or you, ran `repose-notify` on the machine. The body is the message.                                                                                          |
| `todo-app: claude asks`          | An agent ran `repose-ask` and is waiting for your answer. See below.                                                                                                    |
| `todo-app: notifications paused` | The project reached 30 notifications this hour.                                                                                                                         |

The body is what the agent said at that moment, up to 1 KB. It's never your prompt or your terminal. It does pass through ntfy.sh or your email provider, so use a self-hosted ntfy server if that matters.

Claude Code, Codex and opencode report through hooks, within about 10 seconds. Gemini CLI and pi have no hooks, so the machine sends `finished` when their processes go quiet, within about 90 seconds, with the body `gemini went idle` or `pi went idle`.

## Agents can message you and ask questions

Two commands on every machine let an agent reach you directly, from any shell, including its own shell tool:

```
repose-notify "Deploy to staging is green"
repose-ask --options yes,no "Drop the legacy sessions table?"
```

`repose-notify` sends the message and returns at once. `repose-ask` sends the question and waits for your answer, then prints it, so the agent reads it like the output of any other command. You can run them yourself too, for example at the end of a long script.

You can answer a question in four places:

- **ntfy:** when the question has options, the notification has a button for each. Tapping one answers it. No login is needed.
- **Email:** the email has a link for each option. The link opens a page with the question and a button; pressing it answers. Replying to the email doesn't work.
- **The dashboard:** a waiting question shows at the top of the project's page, with a button per option or a text box.
- **Your laptop:** `repose questions` lists the waiting ones and `repose reply` answers:

```
repose questions
repose reply todo-app yes
```

The first answer wins. A link or button used after that says the question was already answered, and what the answer was. **Dismiss** on the dashboard cancels a question without answering.

Tell your agent about the commands in its instructions, for example in `CLAUDE.md`: "When you need a decision from me, run `repose-ask` with the question and use its output as my answer."

`repose-ask` options:

| Option            | Default    |                                                                                   |
| ----------------- | ---------- | --------------------------------------------------------------------------------- |
| `--options A,B,C` | none       | Up to three fixed answers. Without options, any text is an answer.                |
| `--timeout 30m`   | 30 minutes | How long to wait, up to 24 hours. A duration like `90s` or `2h`, or seconds.      |
| `--agent NAME`    | detected   | Who is asking. Usually detected from the agent running the command, else `shell`. |

It exits with:

| Code | Meaning                                                                   |
| ---- | ------------------------------------------------------------------------- |
| 0    | You answered. The answer is on standard output.                           |
| 1    | The machine's agent service couldn't be reached.                          |
| 2    | Wrong usage, such as no question or more than three options.              |
| 3    | No answer before the timeout.                                             |
| 4    | Email is off and there's no ntfy topic, so nobody would see the question. |
| 5    | Cancelled: you dismissed it, the machine stopped, or it restarted.        |
| 130  | Interrupted, for example with `Ctrl-C`. The question is withdrawn.        |

Messages and questions are capped at 1 KB and count toward the project's 30 notifications an hour. The text is shown to you and nobody else, and it isn't written to any log. It does pass through ntfy.sh or your email provider on its way to you.

## Limits

Repeats of the same event from the same agent within 60 seconds are sent once. A project sends at most 30 notifications an hour; past that, one message says they're paused until the next hour.

## Check what happened

```
repose events todo-app
repose events todo-app --since 72h -f
```

If an event is listed but nothing arrived, run `repose notify test`. An `error` there means the channel's settings are wrong.
