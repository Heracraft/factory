# factory

Persistent remote environments for coding agents. `cd` into a project, run
`factory run`, and an agent keeps working in a NixOS microVM after your laptop
closes. Multi-tenant, billed by the hour with a monthly cap, hosted at
`factory.herakraft.co`.

Everything about what this is and how it is built lives in [`docs/`](docs/README.md).
Start there. Contributors and agents also read [`AGENTS.md`](AGENTS.md).

## Layout

```
cmd/         api, hostd, guestd, gateway, factory (CLI), factory-admin
internal/    shared Go; fakes for every interface; generated protobuf under gen/
proto/       gRPC and vsock contracts (docs/interfaces/ is the prose)
nix/         one flake: hosts, edge, guest base, agent overlay, dev shell
infra/       OpenTofu for the Azure-specific pieces and the R2 bucket
apps/web/    SvelteKit dashboard
packages/    TypeScript packages, and packages/core (the v0.01 home-manager flake, being retired)
docs/        the spec
```

## Developing

```
nix develop ./nix     # Go, buf, opentofu, az, just, nixos-anywhere ...
just build
just test
just proto            # regenerate internal/gen from proto/
```

Do not install anything from `nix/` on this machine; hosts and guests are
remote (see `AGENTS.md`).
