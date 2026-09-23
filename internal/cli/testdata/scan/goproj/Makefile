.PHONY: dev lint gen test

BIN := ./tmp/svc

dev:
	air -c .air.toml

lint:
	@golangci-lint run ./...
	-staticcheck ./...

gen: \
    sqlc.yaml
	templ generate && \
	  sqlc generate
	go generate ./...

test:
	go test -race ./...
	$(MAKE) lint
	@echo done > /dev/null
