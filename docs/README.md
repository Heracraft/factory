# factory docs

factory is a service where a developer runs `factory run` in a project directory
and gets a persistent remote environment where coding agents keep working after
the laptop closes. Multi-tenant from the first release, billed from the first
hour, hosted at `factory.herakraft.co` until it graduates to its own domain.

These docs are the source of truth. The code does not exist yet; when code and
docs disagree, the doc is wrong only if a `DECISIONS.md` entry says so.

## Map

| Doc | Read it when |
|---|---|
| [DESIGN.md](DESIGN.md) | You need the whole system in one place. Everything below is a slice of it. |
| [ARCHITECTURE.md](ARCHITECTURE.md) | You need the component map, data flows, network diagram, and repo layout. |
| [DECISIONS.md](DECISIONS.md) | You want to know *why* something is the way it is, or want to change it. Every settled decision, with the alternatives that lost. |
| [MILESTONES.md](MILESTONES.md) | You want to know what to build next and what "done" means for each stage. |
| [CHECKLIST.md](CHECKLIST.md) | You are about to call something finished. The global definition of done. |
| [GLOSSARY.md](GLOSSARY.md) | A word is used in a specific way (guest, host, edge, fragment, closure, project). |
| [RESEARCH.md](RESEARCH.md) | You want the facts and citations the decisions rest on (Azure nested virt, pricing, nixpkgs coverage, Coolify limits, Logto flows, what agents lose remotely). |
| [SECURITY.md](SECURITY.md) | You touch anything that crosses a tenant, host, or network boundary. Threat model and the non-negotiables. |
| [workstreams/](workstreams/README.md) | You are an agent picking up a chunk of work. Each workstream is self-contained: scope, non-goals, interfaces it owns and consumes, and a checklist. |
| [interfaces/](interfaces/README.md) | Two workstreams meet here. gRPC between API and hostd, vsock between hostd and guestd, the HTTP API, the database schema, the SSH gateway login contract, the CLI config file. |
| [features/](features/README.md) | User-facing behaviour, one feature per file, written as the behaviour a user sees and the edge cases that must hold. |
| [ops/RUNBOOK.md](ops/RUNBOOK.md) | Something is broken in production and you need the symptom-to-fix list. |
| [ops/OBSERVABILITY.md](ops/OBSERVABILITY.md) | You are adding a log line, a metric, or a signal that the idle and pricing policies will later depend on. |
| [PRICING.md](PRICING.md) | Tiers, meters, the cost floor per guest, and the trial. |

## How parallel work is organised

The design is split into workstreams that can be built concurrently. Each
workstream document names the interfaces it *owns* (it may change them, and must
update `interfaces/`) and the ones it *consumes* (it codes against the contract
as written and raises a change request in `DECISIONS.md` if the contract is
wrong). The dependency graph and claiming rules are in
[workstreams/README.md](workstreams/README.md).

An agent working a workstream:

1. Reads `DESIGN.md` once, then its workstream doc, then every `interfaces/` doc
   it owns or consumes.
2. Builds the whole workstream, not the parts that are easy. The checklist at the
   end of each workstream doc is the definition of done for that workstream, and
   `CHECKLIST.md` is the definition of done for everything.
3. Records any decision it had to make that the docs did not cover in
   `DECISIONS.md` under "Made during implementation", with the alternatives.
4. Never marks a checklist item done because a build passed. Each item says what
   evidence closes it.

## Conventions

- Prose over bullets where reasoning matters; bullets for parallel items.
- A doc states the failure that a rule prevents. A rule without its failure story
  gets argued with and then deleted.
- Names are fixed: the product, CLI binary, SSH login prefix, config directory and
  Go module are all `factory`. Do not introduce synonyms.
- Dates are absolute (2026-09-17), never "last week".
