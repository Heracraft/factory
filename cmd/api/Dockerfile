# The api and repose-admin image for Coolify (05-control-plane-api.md
# §5.14). Build from the repository root:
#   docker build -f cmd/api/Dockerfile -t repose-api .
# Three stages: the Go build, a Nix stage that collects the closure of
# nix-instantiate for the fragment parse check, and a distroless runtime.

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY proto ./proto
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/repose-admin ./cmd/repose-admin

# nix-instantiate and exactly its runtime closure, nothing else of Nix.
FROM nixos/nix:2.24.10 AS nixtools
RUN mkdir -p /closure/nix/store /closure/usr/local/bin \
 && for p in $(nix-store -qR "$(readlink -f "$(command -v nix-instantiate)")"); do cp -a "$p" /closure/nix/store/; done \
 && ln -s "$(readlink -f "$(command -v nix-instantiate)")" /closure/usr/local/bin/nix-instantiate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=nixtools /closure/nix /nix
COPY --from=nixtools /closure/usr/local/bin/nix-instantiate /usr/local/bin/nix-instantiate
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/repose-admin /usr/local/bin/repose-admin
ENV NIX_INSTANTIATE=/usr/local/bin/nix-instantiate
USER nonroot:nonroot
EXPOSE 8080 8443 8444 9103
# Distroless has no curl or wget for Coolify's own health check, so the
# image carries one: `api -healthcheck` probes the API listener's /healthz
# (http, all) or the metrics listener (grpc). Coolify's rolling deploy
# waits on this (DECISIONS I-87).
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/api", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/api"]
