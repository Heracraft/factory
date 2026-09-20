# Common tasks. Run inside `nix develop ./nix`.

default:
    @just --list

build:
    go build ./...

test:
    go test ./...

lint:
    go vet ./... && golangci-lint run ./... && buf lint

proto:
    buf generate

check-nix:
    nix flake check ./nix

# --- observability (docs/workstreams/10-observability.md) ------------------

# Alert rules, their promtool tests, the dashboards and their queries.
obs-check *ARGS:
    ops/check.sh {{ARGS}}

# The §5 event list against the source: every event, with its call sites, and
# a failure for any event a built component does not emit.
obs-events:
    go test ./internal/obs -run TestEventsEmitted -v

# Rewrite ops/dashboards/*.json from ops/dashboards/gen.py.
dashboards:
    python3 ops/dashboards/gen.py

# Loki, Prometheus, Grafana and Postgres locally, with plausible metrics and
# a database full of synthetic samples. Grafana answers on this machine's
# Tailscale address, port 3000 (folder "repose").
obs-dev:
    docker compose -f ops/dev/docker-compose.yml up -d
    @echo "grafana:    http://$(tailscale ip -4 | head -1):3000"
    @echo "prometheus: http://$(tailscale ip -4 | head -1):9090"
    @echo "metrics:    go run ./ops/dev/seedmetrics"
    @echo "logs:       ops/dev/seedlogs.sh"
    @echo "postgres:   ops/dev/pgcheck.sh"

obs-dev-down:
    docker compose -f ops/dev/docker-compose.yml down -v

# The greps docs/CHECKLIST.md asks for before calling anything done.
done-check paths="cmd internal":
    @echo "-- unhandled errors / panics --"; rg -n '_ = err|panic\(' {{paths}} || true
    @echo "-- leftovers --"; rg -n 'TODO|FIXME|XXX|not implemented' {{paths}} || true
