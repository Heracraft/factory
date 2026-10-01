# Fider at repose.fider.io

The public feedback board (DECISIONS I-403). Fider hosts it on the free
plan (250 suggestions, unlimited voters). Admin:
`https://repose.fider.io/admin`, signed in as `accounts@herakraft.co`.

## Sign-in: the repose account provider

Admin, Authentication, OAuth Providers, `repose account`:

| Field | Value |
| --- | --- |
| Display name | `repose account` (the button reads "Continue with repose account") |
| Logo | the r-mark, `https://repose.herakraft.co/favicon.png` (128 px) |
| Client ID | `wc2np1n3r9z4acp2wutev` (Logto app `repose feedback`) |
| Client secret | Logto, the app, App secrets, Default secret. Not stored anywhere else |
| Authorize URL | `https://accounts.herakraft.co/oidc/auth` |
| Token URL | `https://accounts.herakraft.co/oidc/token` |
| Scope | `openid profile email` |
| Profile API URL | `https://accounts.herakraft.co/oidc/me` |
| JSON path, ID | `sub` |
| JSON path, Name | `name, username` |
| JSON path, Email | `email` |
| Trusted source | No (the site is public) |
| Status | Enabled |

Fider's key for the provider is `_cs7akc6x45`, so its callback, set as the
Logto app's only redirect URI, is
`https://login.fider.io/oauth/_cs7akc6x45/callback`. Fider signs every
board in through `login.fider.io`; a custom domain on the board does not
change the callback.

The Name path leaves out `email` on purpose. An account made with an
email code has no name or username; with `email` in the path Fider would
show the whole address beside the person's posts. Without it Fider uses
the part before the @, and the person can rename themselves in Fider.

The admin page's Test button runs the whole flow and shows the raw
`/oidc/me` body and what Fider parsed from it. If Logto offers a passkey
after sign-in and you go on to the Account Center, the flow never returns
to Fider; run Test again (the Logto session is kept, so GitHub or the
session goes straight through).

## Rotating the secret

Create a new secret on the Logto app, paste it into the provider's Client
secret, Save, run Test, then delete the old secret in Logto. Fider keeps
the stored secret when you save the form without touching that field.

## Still at their defaults

- Fider's own sign-ins: email, Facebook, Google and GitHub are enabled.
  Admins can always use email, whatever the switch says.
- General: no welcome header or message, no invitation text, no logo
  (Fider wants 200 x 200 or more; the mark PNG is 128).
- The Logto app has app-level branding with the mark as its logo and
  nothing else (no terms or privacy links, default colours).
- No custom domain. `feedback.repose.herakraft.co` would need a CNAME
  to Fider and the domain set under General.
