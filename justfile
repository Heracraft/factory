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

# The greps docs/CHECKLIST.md asks for before calling anything done.
done-check paths="cmd internal":
    @echo "-- unhandled errors / panics --"; rg -n '_ = err|panic\(' {{paths}} || true
    @echo "-- leftovers --"; rg -n 'TODO|FIXME|XXX|not implemented' {{paths}} || true
