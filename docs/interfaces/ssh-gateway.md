# SSH gateway contract

## Names

- Gateway address: `ssh.repose.herakraft.co:22`.
- Login name: `<project-slug>.<user-handle>`, e.g. `todo-app.heracraft`.
  The gateway splits on the last `.`.
- Inside the guest the Unix user is always `dev`.

## Certificates

Two CAs, both ed25519, private keys in the api's secret store:

- **User CA** signs user certificates. `POST /certs` returns a certificate
  for the user's public key with: `principals = [project_id...]`, `valid
  after now-1m`, `valid before now+12h`, `serial` = monotonic bigint,
  `key_id = "<user_id>:<handle>"`, extensions `permit-pty`,
  `permit-port-forwarding`, `permit-agent-forwarding`. No
  `source-address` restriction (laptops move).
- **Host CA** signs the gateway's host key and every guest's host key
  (guest keys are generated at create by hostd and signed via the api).
  The CLI writes `@cert-authority ssh.repose.herakraft.co,10.64.* <host ca>`
  to `~/.ssh/repose/known_hosts`, so there is never a host-key prompt.

## Gateway behaviour

1. Accept the TCP connection, present the host certificate.
2. Public-key auth only. Verify the offered certificate: signed by User CA,
   within validity, serial not in the revocation set (refreshed from
   `/internal/revoked` every 30 s, plus a push on revoke).
3. Resolve `login` via `GET /internal/route`. If the project's `state` is
   not `running`, reject with a banner: `todo-app is stopped; run \`repose
   start\``. If the certificate's principals do not contain the project id,
   reject with `certificate not valid for this project`.
4. Terminate the client's SSH session at the gateway, then open a second
   SSH session to `guest_ip:22` over WireGuard and relay channels between
   the two (session, `direct-tcpip` for `-L`, `auth-agent@openssh.com` for
   agent forwarding, pty requests, window changes, signals, exit status).
   The gateway authenticates to the guest with a **gateway-issued
   certificate**: 5-minute validity, principal = project id, `key_id`
   suffixed `:via-gateway`, signed by the User CA with the gateway's own key
   pair. The guest's sshd therefore remains a second, independent check.
   Raw TCP relay after auth was rejected because the login name is only
   known after the client's key exchange with the gateway completes.
5. Report `POST /internal/sessions` on open and close.
6. Port forwards (`-L`) and agent forwarding are passed through.

## Guest sshd

```
TrustedUserCAKeys /etc/ssh/user_ca.pub
HostCertificate /etc/ssh/ssh_host_ed25519_key-cert.pub
AuthorizedPrincipalsFile /etc/ssh/principals/%u     # contains the project id
PasswordAuthentication no
PermitRootLogin no
AllowUsers dev
```

## CLI side

`~/.ssh/repose/config` (included from `~/.ssh/config` by a line the CLI adds
once, `Include ~/.ssh/repose/config`):

```
Host todo-app.repose
  HostName ssh.repose.herakraft.co
  User todo-app.heracraft
  CertificateFile ~/.ssh/repose/id_ed25519-cert.pub
  IdentityFile ~/.ssh/id_ed25519
  UserKnownHostsFile ~/.ssh/repose/known_hosts
  ForwardAgent yes
  ServerAliveInterval 30
```

So `ssh todo-app.repose` works from any tool (VS Code Remote-SSH, Zed,
Cursor) without the CLI, as long as the certificate is fresh. `repose run`
refreshes it.

## Test CA

`internal/ca/testca` generates both CAs in memory and signs certificates for
tests. Gateway and guest sshd tests use it.
